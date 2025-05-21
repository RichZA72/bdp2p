package fs

import (
	"fmt"
	"os"
	"time"
	"encoding/json"
	"net"

	"p2pfs/internal/peer"
)

type FileInfo struct {
	Name    string    `json:"name"`
	ModTime time.Time `json:"modTime"`
}

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

func GetRemoteFiles(ip, port string) ([]FileInfo, error) {
	address := fmt.Sprintf("%s:%s", ip, port)
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return nil, err
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
		rawFiles := response["files"].([]interface{})
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

func GetFilesByPeer(p peer.PeerInfo, localID int) ([]FileInfo, error) {
	if p.ID == localID {
		return GetLocalFiles()
	}
	return GetRemoteFiles(p.IP, p.Port)
}
