package peer

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// FileInfo representa un archivo disponible en el nodo.
type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
}



// Maneja las solicitudes entrantes como GET_FILES
func handleConnection(conn net.Conn) {
	defer conn.Close()

	var msg map[string]interface{}
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&msg); err != nil {
		fmt.Println("Error al decodificar mensaje:", err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		fmt.Println("Mensaje sin tipo válido")
		return
	}

	switch msgType {
	case "GET_FILES":
		files := []FileInfo{}
		dir := "shared"
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Println("Error leyendo carpeta shared:", err)
			return
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

		response := map[string]interface{}{
			"type":  "FILES_LIST",
			"files": files,
		}

		encoder := json.NewEncoder(conn)
		encoder.Encode(response)
	}
}
