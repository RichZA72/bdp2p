package peer

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// Estructura del archivo a enviar en la respuesta
type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
}

// Inicia el servidor TCP que escucha en el puerto indicado
func StartServer(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("❌ Error iniciando servidor TCP:", err)
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

// Maneja cada conexión entrante
func handleConnection(conn net.Conn) {
	defer conn.Close()

	var request map[string]string
	err := json.NewDecoder(conn).Decode(&request)
	if err != nil {
		fmt.Println("⚠️ Error al decodificar solicitud:", err)
		return
	}

	switch request["type"] {
	case "GET_FILES":
		files, err := getLocalFiles()
		if err != nil {
			fmt.Println("⚠️ Error obteniendo archivos locales:", err)
			return
		}
		response := map[string]interface{}{
			"type":  "FILES_LIST",
			"files": files,
		}
		err = json.NewEncoder(conn).Encode(response)
		if err != nil {
			fmt.Println("⚠️ Error al enviar respuesta:", err)
		}

	default:
		fmt.Println("❌ Solicitud desconocida:", request["type"])
	}
}

// Obtiene los archivos locales desde la carpeta shared/
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
