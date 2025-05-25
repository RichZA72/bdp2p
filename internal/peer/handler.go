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

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fmt.Println("❌ Error al decodificar archivo:", err)
		return
	}

	err = os.WriteFile(filepath.Join("shared", name), data, 0644)
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

// getLocalFiles devuelve los archivos disponibles en el nodo local
func getLocalFiles() ([]FileInfo, error) {
	var files []FileInfo
	dir := "shared"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, FileInfo{
				Name:    entry.Name(),
				ModTime: info.ModTime(),
			})
		}
	}
	return files, nil
}

// handleSyncLogs aplica los cambios enviados en los logs si son dirigidos a esta máquina
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

		// Solo aplicar si el destino soy yo
		if targetID != Local.ID {
			continue
		}

		switch action {
		case "DELETE":
			err := os.Remove(filepath.Join("shared", fileName))
			if err == nil {
				fmt.Println("🗑️ Archivo eliminado por log:", fileName)
			} else {
				fmt.Println("⚠️ No se pudo eliminar (puede que ya no exista):", fileName)
			}

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

// requestFileFromPeer solicita un archivo a otro nodo y lo guarda localmente
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
	os.WriteFile(filepath.Join("shared", filename), decoded, 0644)
}

// SendSyncLog envía una operación a los demás nodos como log de sincronización
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
