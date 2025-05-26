package fs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"p2pfs/internal/peer"
	"p2pfs/internal/state"
)

// ResyncAfterReconnect aplica operaciones pendientes a un nodo recién reconectado
func ResyncAfterReconnect(peerID int) {
	fmt.Printf("🔄 ResyncAfterReconnect: ejecutando para nodo %d\n", peerID)

	ops := state.GetAndClearPendingOps(peerID)
	peers := peer.GetPeers()
	localID := peer.Local.ID

	// Mapa para acceso rápido por ID
	peerMap := make(map[int]peer.PeerInfo)
	for _, p := range peers {
		peerMap[p.ID] = p
	}

	target, ok := peerMap[peerID]
	if !ok {
		fmt.Printf("⚠️ Peer %d no encontrado\n", peerID)
		return
	}

	for _, op := range ops {
		switch op.Type {
		case "send":
			// Enviar archivo si este nodo es el origen
			if op.SourceID == localID {
				err := SendFileToPeer(target, op.FilePath)
				if err != nil {
					fmt.Printf("❌ Error al reenviar archivo a %s: %v\n", target.IP, err)
				}
			}
		case "get":
			// Solicitar archivo si este nodo es el destino
			if op.TargetID == localID {
				err := RequestFileFromPeer(target, op.FilePath, op.Flatten)
				if err != nil {
					fmt.Printf("❌ Error al solicitar archivo %s: %v\n", op.FilePath, err)
				}
			}
		case "delete":
			// Enviar solicitud de eliminación si este nodo es el origen
			if op.SourceID == localID {
				go sendDeleteRequest(target, op.FilePath)
			}
		default:
			fmt.Printf("⚠️ Operación desconocida: %s\n", op.Type)
		}
	}
}



func requestFileListFromPeer(address string) ([]state.FileInfo, error) {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := map[string]interface{}{"type": "GET_FILES"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	if resp["type"] != "FILES_LIST" {
		return nil, fmt.Errorf("respuesta inesperada: %v", resp["type"])
	}

	rawList, _ := json.Marshal(resp["files"])
	var files []state.FileInfo
	json.Unmarshal(rawList, &files)
	return files, nil
}

func requestFileFromPeer(ip, filename string) {
	conn, err := net.Dial("tcp", ip+":9000")
	if err != nil {
		fmt.Println("❌ No se pudo conectar a", ip)
		return
	}
	defer conn.Close()

	req := map[string]interface{}{"type": "GET_FILE", "name": filename}
	json.NewEncoder(conn).Encode(req)

	var resp map[string]interface{}
	err = json.NewDecoder(conn).Decode(&resp)
	if err != nil || resp["type"] != "FILE_CONTENT" {
		fmt.Println("❌ Error al recibir archivo:", err)
		return
	}

	data, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
	path := filepath.Join("shared", filename)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, data, 0644)
}

// StartAutoSync sincroniza periódicamente con los peers
func StartAutoSync(peerSystem *peer.Peer, localID int, callbacks SyncCallbacks) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			for _, pinfo := range peerSystem.Peers {
				var files []state.FileInfo
				isOnline := true

				if pinfo.ID != localID {
					var err error
					address := fmt.Sprintf("%s:%s", pinfo.IP, pinfo.Port)
					files, err = requestFileListFromPeer(address)
					isOnline = err == nil
				} else {
					files = ListSharedFiles()
				}

				wasOnline := state.OnlineStatus[pinfo.IP]
				state.OnlineStatus[pinfo.IP] = isOnline

				if isOnline && !wasOnline && pinfo.ID != localID {
					ResyncAfterReconnect(pinfo.ID)
				}

				if isOnline {
					state.FileCache[pinfo.IP] = files
					callbacks.UpdateStatus(pinfo.ID, true)
					callbacks.UpdateFileList(pinfo.ID, files)
				} else {
					callbacks.UpdateStatus(pinfo.ID, false)
					callbacks.UpdateFileList(pinfo.ID, state.FileCache[pinfo.IP])
				}
			}
		}
	}()
}

func ListSharedFiles() []state.FileInfo {
	var files []state.FileInfo
	_ = filepath.Walk("shared", func(path string, info os.FileInfo, err error) error {
		if err != nil || path == "shared" {
			return nil
		}
		rel, _ := filepath.Rel("shared", path)
		files = append(files, state.FileInfo{
			Name:    rel,
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
		return nil
	})
	return files
}

func GetLocalOrRemoteFileList(peerSystem *peer.Peer, peerID int) ([]state.FileInfo, error) {
	if peerID == peerSystem.Local.ID {
		return ListSharedFiles(), nil
	}
	for _, p := range peerSystem.Peers {
		if p.ID == peerID {
			return requestFileListFromPeer(fmt.Sprintf("%s:%s", p.IP, p.Port))
		}
	}
	return nil, fmt.Errorf("peer %d no encontrado", peerID)
}

type SyncCallbacks struct {
	UpdateStatus   func(peerID int, online bool)
	UpdateFileList func(peerID int, files []state.FileInfo)
}
