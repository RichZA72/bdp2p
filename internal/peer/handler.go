package peer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
	"strings"
	"p2pfs/internal/state"
)

var Local PeerInfo
var Peers []PeerInfo

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

func handleGetFiles(conn net.Conn) {
	files, err := getLocalFiles()
	if err != nil {
		fmt.Println("❌ No se pudieron listar archivos:", err)
		return
	}
	fmt.Println("📦 Enviando lista de archivos:", len(files))
	resp := map[string]interface{}{
		"type":  "FILES_LIST",
		"files": files,
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func handleSendFile(conn net.Conn, name string) {
	path := filepath.Join("shared", filepath.Clean(name))
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("❌ No se pudo acceder al archivo '%s': %v\n", path, err)
		resp := map[string]interface{}{
			"type":  "ERROR",
			"error": fmt.Sprintf("Archivo no accesible: %v", err),
		}
		_ = json.NewEncoder(conn).Encode(resp)
		return
	}

	if info.IsDir() {
		resp := map[string]interface{}{
			"type":  "ERROR",
			"error": "No se puede enviar una carpeta como archivo",
		}
		_ = json.NewEncoder(conn).Encode(resp)
		fmt.Println("⚠️ Se intentó enviar un directorio como archivo:", name)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("❌ Error al leer el archivo '%s': %v\n", path, err)
		resp := map[string]interface{}{
			"type":  "ERROR",
			"error": fmt.Sprintf("Lectura fallida: %v", err),
		}
		_ = json.NewEncoder(conn).Encode(resp)
		return
	}

	resp := map[string]interface{}{
		"type":    "FILE_CONTENT",
		"name":    name,
		"content": base64.StdEncoding.EncodeToString(data),
	}
	_ = json.NewEncoder(conn).Encode(resp)
	fmt.Println("📤 Archivo enviado correctamente:", name)
}


func handleReceiveFile(request map[string]interface{}) {
	name, ok1 := request["name"].(string)
	content, ok2 := request["content"].(string)
	isDir, _ := request["isDir"].(bool)

	// ✅ Nuevo comportamiento:
	// Si el nombre tiene separadores de ruta, se considera con estructura.
	hasPath := strings.Contains(name, "/") || strings.Contains(name, "\\")

	var path string
	if isDir {
		path = filepath.Join("shared", name)
		if err := os.MkdirAll(path, 0755); err != nil {
			fmt.Println("❌ Error al crear carpeta recibida:", err)
		} else {
			fmt.Println("📁 Carpeta recibida:", name)
		}
		return
	}

	if !ok1 || !ok2 {
		fmt.Println("❌ Formato inválido en archivo recibido")
		return
	}

	// Si tiene subruta, respeta la estructura. Si no, lo guarda directo.
	if hasPath {
		path = filepath.Join("shared", name)
	} else {
		path = filepath.Join("shared", filepath.Base(name))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Println("❌ Error al crear carpeta destino:", err)
		return
	}

	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		fmt.Println("❌ Error al decodificar archivo:", err)
		return
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		fmt.Println("❌ Error al guardar archivo recibido:", err)
		return
	}

	fmt.Println("📥 Archivo recibido y guardado:", path)
}



func handleDeleteFile(conn net.Conn, name string) {
	path := filepath.Join("shared", name)
	info, err := os.Stat(path)

	status := "ok"

	if err != nil {
		fmt.Println("❌ No se pudo acceder a", name, ":", err)
		status = "error"
	} else {
		if info.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			fmt.Println("❌ Error al eliminar", name, ":", err)
			status = "error"
		} else {
			fmt.Println("🗑️ Eliminado:", name)
		}
	}

	resp := map[string]interface{}{
		"type":   "DELETE_ACK",
		"status": status,
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func getLocalFiles() ([]state.FileInfo, error) {
	var files []state.FileInfo
	dir := "shared"

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, state.FileInfo{
			Name:    rel,
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
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
	if err != nil {
		fmt.Println("❌ Error al recibir archivo:", err)
		return
	}

	if resp["type"] != "FILE_CONTENT" {
		errMsg, _ := resp["error"].(string)
		fmt.Printf("❌ Error del peer remoto: %v\n", errMsg)
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
