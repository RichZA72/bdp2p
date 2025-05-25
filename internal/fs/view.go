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

// ✅ Obtiene archivos locales del directorio "shared"
func GetLocalFiles() ([]state.FileInfo, error) {
	var files []state.FileInfo
	dir := "shared"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error leyendo carpeta local: %v", err)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, state.FileInfo{
			Name:    entry.Name(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
		})
	}
	return files, nil
}

// ✅ Solicita archivos a un nodo remoto
func GetRemoteFiles(ip, port string) ([]state.FileInfo, error) {
	address := fmt.Sprintf("%s:%s", ip, port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
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

	var result []state.FileInfo

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
			f, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			// Extracción segura de campos
			name, nameOK := f["name"].(string)
			modStr, modOK := f["modTime"].(string)
			isDir, _ := f["isDir"].(bool) // por defecto false

			modTime := time.Now()
			if modOK {
				if parsed, err := time.Parse(time.RFC3339, modStr); err == nil {
					modTime = parsed
				}
			}

			if nameOK {
				result = append(result, state.FileInfo{
					Name:    name,
					ModTime: modTime,
					IsDir:   isDir,
				})
			}
		}
	}

	return result, nil
}

// ✅ Retorna archivos del nodo especificado
func GetFilesByPeer(p peer.PeerInfo, localID int) ([]state.FileInfo, error) {
	if p.ID == localID {
		return GetLocalFiles()
	}

	files, err := GetRemoteFiles(p.IP, p.Port)
	if err != nil {
		state.AddPendingOp(p.ID, state.PendingOperation{
			Type:     "get",
			FilePath: "",
			TargetID: -1,
			SourceID: localID,
		})
		return nil, err
	}
	return files, nil
}

// ✅ Compara archivos locales con los del nodo remoto y retorna los que faltan o están desactualizados
func CompararArchivos(localFiles, remotoFiles []state.FileInfo) []state.FileInfo {
	var faltantes []state.FileInfo
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
