package peer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Variables globales asignadas externamente
var Local PeerInfo
var Peers []PeerInfo

type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
	IsDir   bool      `json:"isDir"` // ← Agregado
}

// StartServer inicia el servidor TCP en el puerto indicado
func StartServer(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("❌ Error iniciando servidor:", err)
		return
	}
	fmt.Println("🟢 Servidor TCP escuchando en el puerto", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("⚠️ Error al aceptar conexión:", err)
			continue
		}
		go handleConnection(conn)
	}
}

// handleConnection gestiona los mensajes entrantes
func handleConnection(conn net.Conn) {
	defer conn.Close()

	var request map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		fmt.Println("⚠️ Error al decodificar mensaje:", err)
		return
	}

	switch t := request["type"].(string); t {
	case "GET_FILES":
		handleGetFiles(conn)

	case "GET_FILE":
		name, ok := request["name"].(string)
		if ok {
			handleSendFile(conn, name)
		}

	case "SEND_FILE":
		handleReceiveFile(request)

	case "DELETE_FILE":
		name, ok := request["name"].(string)
		if ok {
			handleDeleteFile(conn, name)
		}

	case "DELETE_DIR":
		name, ok := request["name"].(string)
		if ok {
			handleDeleteDir(conn, name)
		}

	case "SYNC_LOGS":
		handleSyncLogs(request)

	default:
		fmt.Println("⚠️ Tipo de mensaje desconocido:", request["type"])
	}
}

// --- HANDLERS ---

func handleGetFiles(conn net.Conn) {
	files, err := getLocalFiles()
	if err != nil {
		fmt.Println("❌ No se pudieron listar archivos:", err)
		return
	}
	resp := map[string]interface{}{
		"type":  "FILES_LIST",
		"files": files,
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func handleSendFile(conn net.Conn, name string) {
	path := filepath.Join("shared", name)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("❌ No se pudo leer el archivo:", err)
		return
	}

	resp := map[string]interface{}{
		"type":    "FILE_CONTENT",
		"name":    name,
		"content": base64.StdEncoding.EncodeToString(data),
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func handleReceiveFile(request map[string]interface{}) {
	name, ok1 := request["name"].(string)
	encoded, ok2 := request["content"].(string)
	if !ok1 || !ok2 {
		fmt.Println("❌ Formato inválido en archivo recibido")
		return
	}

	path := filepath.Join("shared", name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Println("❌ Error al crear carpeta destino:", err)
		return
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fmt.Println("❌ Error al decodificar archivo:", err)
		return
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		fmt.Println("❌ Error al guardar archivo recibido:", err)
		return
	}

	fmt.Println("📥 Archivo recibido y guardado:", name)
}

func handleDeleteFile(conn net.Conn, name string) {
	err := os.Remove(filepath.Join("shared", name))
	status := "ok"
	if err != nil {
		fmt.Println("❌ Error al eliminar archivo:", err)
		status = "error"
	}
	resp := map[string]interface{}{
		"type":   "DELETE_ACK",
		"status": status,
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func handleDeleteDir(conn net.Conn, name string) {
	err := os.RemoveAll(filepath.Join("shared", name))
	status := "ok"
	if err != nil {
		fmt.Println("❌ Error al eliminar carpeta:", err)
		status = "error"
	} else {
		fmt.Println("🗑️ Carpeta eliminada:", name)
	}
	resp := map[string]interface{}{
		"type":   "DELETE_ACK",
		"status": status,
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func getLocalFiles() ([]FileInfo, error) {
	var files []FileInfo
	dir := "shared"

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, FileInfo{
		Name: rel, 
		ModTime: info.ModTime(), 
		IsDir:info.IsDir(), })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func handleSyncLogs(request map[string]interface{}) {
	rawLogs, ok := request["logs"].([]interface{})
	if !ok {
		fmt.Println("❌ Formato inválido de logs.")
		return
	}

	for _, item := range rawLogs {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		action, _ := entry["action"].(string)
		fileName, _ := entry["fileName"].(string)
		originID := int(entry["originID"].(float64))
		targetID := int(entry["targetID"].(float64))

		if targetID != Local.ID {
			continue
		}

		switch action {
		case "DELETE":
			_ = os.RemoveAll(filepath.Join("shared", fileName))
			fmt.Println("🗑️ Eliminado por log:", fileName)

		case "TRANSFER":
			for _, peer := range Peers {
				if peer.ID == originID {
					go requestFileFromPeer(peer, fileName)
					break
				}
			}

		default:
			fmt.Println("⚠️ Acción no reconocida:", action)
		}
	}
}

func requestFileFromPeer(peer PeerInfo, filename string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
	if err != nil {
		fmt.Println("❌ No se pudo conectar a", peer.IP)
		return
	}
	defer conn.Close()

	req := map[string]interface{}{
		"type": "GET_FILE",
		"name": filename,
	}
	_ = json.NewEncoder(conn).Encode(req)

	var resp map[string]interface{}
	err = json.NewDecoder(conn).Decode(&resp)
	if err != nil || resp["type"] != "FILE_CONTENT" {
		fmt.Println("❌ Error al recibir archivo:", err)
		return
	}

	decoded, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
	if err := os.MkdirAll(filepath.Dir(filepath.Join("shared", filename)), 0755); err != nil {
		fmt.Println("❌ Error creando carpeta para archivo recibido:", err)
		return
	}
	_ = os.WriteFile(filepath.Join("shared", filename), decoded, 0644)
}

func SendSyncLog(action, fileName string, originID, targetID int) {
	log := map[string]interface{}{
		"type": "SYNC_LOGS",
		"logs": []map[string]interface{}{
			{
				"action":   action,
				"fileName": fileName,
				"originID": originID,
				"targetID": targetID,
			},
		},
	}

	for _, peer := range Peers {
		if peer.ID == Local.ID || peer.ID == targetID {
			continue
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port), 2*time.Second)
		if err != nil {
			continue
		}
		_ = json.NewEncoder(conn).Encode(log)
		conn.Close()
	}
}
