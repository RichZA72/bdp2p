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
		switch op.Type {
		case "send", "get":
			targetPeer, exists := peerMap[op.TargetID]
			if !exists {
				fmt.Printf("⚠️ Nodo destino %d no encontrado\n", op.TargetID)
				continue
			}

			filePath := filepath.Join("shared", op.FilePath)
			if _, err := os.Stat(filePath); err == nil {
				err := SendFileToPeer(targetPeer, op.FilePath)
				if err != nil {
					fmt.Printf("❌ Error al enviar '%s' a nodo %d: %v\n", op.FilePath, op.TargetID, err)
				} else {
					fmt.Printf("📤 '%s' reenviado tras reconexión\n", op.FilePath)
					peer.SendSyncLog("TRANSFER", op.FilePath, peerID, op.TargetID)
				}
			} else if localID == op.SourceID {
				sourcePeer := peerMap[op.SourceID]
				err := RelayFileBetweenPeers(sourcePeer, op.FilePath, []peer.PeerInfo{targetPeer})
				if err != nil {
					fmt.Printf("❌ Relay fallido para '%s': %v\n", op.FilePath, err)
				} else {
					fmt.Printf("📥 Relay de '%s' realizado a %d\n", op.FilePath, op.TargetID)
				}
			} else {
				fmt.Printf("⚠️ Archivo '%s' no encontrado localmente y no soy SourceID\n", op.FilePath)
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

func requestFileListFromPeer(ip string) ([]state.FileInfo, error) {
	conn, err := net.DialTimeout("tcp", ip+":9000", 2*time.Second)
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

type SyncCallbacks struct {
	UpdateStatus   func(peerID int, online bool)
	UpdateFileList func(peerID int, files []state.FileInfo)
}

func StartAutoSync(peerSystem *peer.Peer, localID int, callbacks SyncCallbacks) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			for _, pinfo := range peerSystem.Peers {
				var files []state.FileInfo
				isOnline := true

				if pinfo.ID != localID {
					var err error
					var tmp []state.FileInfo
					tmp, err = GetFilesByPeer(pinfo, localID)
					isOnline = err == nil
					if isOnline {
						files = tmp // ✅ No se necesita conversión si tmp es del tipo correcto
					}
				} else {
					files = listSharedFiles()
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

func listSharedFiles() []state.FileInfo {
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

