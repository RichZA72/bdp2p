package fs

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"p2pfs/internal/peer"
	"p2pfs/internal/state"
)

type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
}

// ✅ Obtiene archivos locales del directorio "shared"
func GetLocalFiles() ([]FileInfo, error) {
	var files []FileInfo
	dir := "shared"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error leyendo carpeta local: %v", err)
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

// ✅ Solicita archivos a un nodo remoto
func GetRemoteFiles(ip, port string) ([]FileInfo, error) {
	address := fmt.Sprintf("%s:%s", ip, port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		// Si falla la conexión, marcamos como desconectado y retornamos error
		state.OnlineStatus[ip] = false
		return nil, fmt.Errorf("nodo %s desconectado", ip)
	}
	defer conn.Close()

	request := map[string]string{
		"type": "GET_FILES",
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, err
	}

	var response map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return nil, err
	}

	var result []FileInfo

	if response["type"] == "FILES_LIST" {
		raw, ok := response["files"]
		if !ok {
			fmt.Println("❌ 'files' no encontrado en la respuesta")
			return result, nil
		}

		rawFiles, ok := raw.([]interface{})
		if !ok || rawFiles == nil {
			fmt.Println("❌ 'files' no es una lista válida o es nil")
			return result, nil
		}

		for _, item := range rawFiles {
			f := item.(map[string]interface{})
			modTime, _ := time.Parse(time.RFC3339, f["modTime"].(string))
			result = append(result, FileInfo{
				Name:    f["name"].(string),
				ModTime: modTime,
			})
		}
	}

	return result, nil
}

// ✅ Retorna archivos del nodo especificado
func GetFilesByPeer(p peer.PeerInfo, localID int) ([]FileInfo, error) {
	if p.ID == localID {
		return GetLocalFiles()
	}

	files, err := GetRemoteFiles(p.IP, p.Port)
	if err != nil {
		// Marcar como operación pendiente si está desconectado
		state.AddPendingOp(p.ID, state.PendingOperation{
		Type:     "get",
		FilePath: "",     // Este campo puede quedar así si no aplica
		TargetID: -1,     // ✅ Usa un valor dummy como -1 para indicar "no aplica"
		})

		return nil, err
	}
	return files, nil
}

// ✅ Compara archivos locales con los del nodo remoto y retorna los que faltan o están desactualizados
func CompararArchivos(localFiles, remotoFiles []FileInfo) []FileInfo {
	var faltantes []FileInfo
	remotoMap := make(map[string]time.Time)

	for _, rf := range remotoFiles {
		remotoMap[rf.Name] = rf.ModTime
	}

	for _, lf := range localFiles {
		if t, ok := remotoMap[lf.Name]; !ok || lf.ModTime.After(t) {
			faltantes = append(faltantes, lf)
		}
	}

	return faltantes
}
