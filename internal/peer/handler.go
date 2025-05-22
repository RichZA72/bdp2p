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

type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
}

// StartServer inicia el servidor TCP en el puerto local
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

// handleConnection maneja solicitudes entrantes
func handleConnection(conn net.Conn) {
	defer conn.Close()

	var request map[string]interface{}
	err := json.NewDecoder(conn).Decode(&request)
	if err != nil {
		fmt.Println("⚠️ Error al decodificar mensaje:", err)
		return
	}

	switch request["type"] {
	case "GET_FILES":
		handleGetFiles(conn)

	case "GET_FILE":
		name := request["name"].(string)
		handleSendFile(conn, name)

	case "SEND_FILE":
		handleReceiveFile(request)

	case "DELETE_FILE":
		name := request["name"].(string)
		handleDeleteFile(conn, name)
	}
}

// --- HANDLERS ---

func handleGetFiles(conn net.Conn) {
	files, err := getLocalFiles()
	if err != nil {
		return
	}
	resp := map[string]interface{}{
		"type":  "FILES_LIST",
		"files": files,
	}
	json.NewEncoder(conn).Encode(resp)
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
	json.NewEncoder(conn).Encode(resp)
}

func handleReceiveFile(request map[string]interface{}) {
	name := request["name"].(string)
	encoded := request["content"].(string)

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
	json.NewEncoder(conn).Encode(resp)
}

// getLocalFiles devuelve info de los archivos locales
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
