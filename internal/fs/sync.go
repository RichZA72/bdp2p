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
		relPath := filepath.Clean(op.FilePath)

		switch op.Type {

		case "get":
			if op.TargetID == localID {
				// Yo solicité un archivo: pedirlo al SourceID
				sourcePeer, exists := peerMap[op.SourceID]
				if !exists {
					fmt.Printf("⚠️ Source peer %d no encontrado para obtener '%s'\n", op.SourceID, relPath)
					continue
				}
				err := RequestFileFromPeer(sourcePeer, relPath)
				if err != nil {
					fmt.Printf("❌ Error al obtener '%s' desde nodo %d: %v\n", relPath, op.SourceID, err)
				} else {
					fmt.Printf("📥 '%s' recibido tras reconexión desde %d\n", relPath, op.SourceID)
				}
			}

		case "send":
			targetPeer, exists := peerMap[op.TargetID]
			if !exists {
				fmt.Printf("⚠️ Nodo destino %d no encontrado\n", op.TargetID)
				continue
			}

			localFile := filepath.Join("shared", relPath)
			if _, err := os.Stat(localFile); err == nil {
				err := SendFileToPeer(targetPeer, relPath)
				if err != nil {
					fmt.Printf("❌ Error al reenviar '%s' a nodo %d: %v\n", relPath, op.TargetID, err)
				} else {
					fmt.Printf("📤 '%s' reenviado tras reconexión\n", relPath)
					peer.SendSyncLog("TRANSFER", relPath, localID, op.TargetID)
				}
			} else if localID == op.SourceID {
				sourcePeer := peerMap[op.SourceID]
				err := RelayFileBetweenPeers(sourcePeer, relPath, []peer.PeerInfo{targetPeer})
				if err != nil {
					fmt.Printf("❌ Relay fallido para '%s': %v\n", relPath, err)
				} else {
					fmt.Printf("📥 Relay de '%s' realizado a %d\n", relPath, op.TargetID)
				}
			} else {
				fmt.Printf("⚠️ Archivo '%s' no disponible y no soy SourceID\n", relPath)
			}

		case "delete":
			sendDeleteRequest(target, op.FilePath)
			fmt.Printf("🗑️ Eliminación reenviada tras reconexión: %s\n", op.FilePath)
			peer.SendSyncLog("DELETE", op.FilePath, localID, peerID)

		default:
			fmt.Printf("⚠️ Tipo de operación desconocido: %s\n", op.Type)
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
