package fs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"p2pfs/internal/state"
	"p2pfs/internal/peer"
)

// 🆕 Aplica operaciones pendientes al reconectarse un nodo
func ResyncAfterReconnect(peerID int) {
	fmt.Printf("🔄 ResyncAfterReconnect: ejecutando para nodo %d\n", peerID)

	ops := state.GetAndClearPendingOps(peerID)
	peers := peer.GetPeers()

	// Mapa rápido de ID a PeerInfo
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
			// Si yo soy el que envía
			if op.SourceID == -1 || op.SourceID == peer.GetPeers()[0].ID {
				err := SendFileToPeer(target, op.FilePath)
				if err != nil {
					fmt.Printf("❌ Error reenviando archivo pendiente: %s → %v\n", op.FilePath, err)
				} else {
					fmt.Printf("📤 Archivo reenviado tras reconexión: %s\n", op.FilePath)
					peer.SendSyncLog("TRANSFER", op.FilePath, peer.GetPeers()[0].ID, peerID)
				}
			} else {
				origin, exists := peerMap[op.SourceID]
				if exists {
					fmt.Printf("📥 Solicitando archivo %s desde %s para %s (relay)\n", op.FilePath, origin.IP, target.IP)
					err := RelayFileBetweenPeers(origin, op.FilePath, []peer.PeerInfo{target})
					if err != nil {
						fmt.Printf("❌ Error en relay: %v\n", err)
					}
				}
			}
		case "get":
			if op.SourceID != -1 {
				requester, exists := peerMap[op.SourceID]
				if exists {
					fmt.Printf("📤 Enviando archivo %s a %s que lo pidió mientras yo estaba desconectado\n", op.FilePath, requester.IP)
					err := SendFileToPeer(requester, op.FilePath)
					if err != nil {
						fmt.Printf("❌ Error al enviar archivo tras reconexión: %v\n", err)
					} else {
						peer.SendSyncLog("TRANSFER", op.FilePath, peerID, requester.ID)
					}
				}
			}
		case "delete":
			sendDeleteRequest(target, op.FilePath)
			fmt.Printf("🗑️ Eliminación reenviada tras reconexión: %s\n", op.FilePath)
			peer.SendSyncLog("DELETE", op.FilePath, peer.GetPeers()[0].ID, peerID)
		default:
			fmt.Println("⚠️ Operación desconocida:", op.Type)
		}
	}
}

func fileExistsLocally(name string, list []FileInfo) bool {
	for _, f := range list {
		if f.Name == name {
			return true
		}
	}
	return false
}

func fileInList(name string, list []FileInfo) bool {
	for _, f := range list {
		if f.Name == name {
			return true
		}
	}
	return false
}

func requestFileListFromPeer(ip string) ([]FileInfo, error) {
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
	var files []FileInfo
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
	if err := json.NewDecoder(conn).Decode(&resp); err != nil || resp["type"] != "FILE_CONTENT" {
		fmt.Println("❌ Error al recibir archivo:", err)
		return
	}

	data, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
	os.WriteFile(filepath.Join("shared", filename), data, 0644)
}

// SyncCallbacks contiene funciones que la GUI pasa para actualizar visualmente
type SyncCallbacks struct {
	UpdateStatus   func(peerID int, online bool)
	UpdateFileList func(peerID int, files []state.FileInfo)
}

// StartAutoSync inicia la sincronización automática entre nodos
func StartAutoSync(peerSystem *peer.Peer, localID int, callbacks SyncCallbacks) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			for _, pinfo := range peerSystem.Peers {
				files, err := GetFilesByPeer(pinfo, localID)
				isOnline := err == nil
				wasOnline := state.OnlineStatus[pinfo.IP]
				state.OnlineStatus[pinfo.IP] = isOnline

				// 🆕 Si se reconectó un nodo remoto → sincronizar pendientes
				if isOnline && !wasOnline && pinfo.ID != localID {
					ResyncAfterReconnect(pinfo.ID)
				}

				if isOnline {
					converted := make([]state.FileInfo, 0, len(files))
					for _, f := range files {
						converted = append(converted, state.FileInfo{Name: f.Name, ModTime: f.ModTime})
					}
					state.FileCache[pinfo.IP] = converted
					callbacks.UpdateStatus(pinfo.ID, true)
					callbacks.UpdateFileList(pinfo.ID, converted)
				} else {
					callbacks.UpdateStatus(pinfo.ID, false)
					callbacks.UpdateFileList(pinfo.ID, state.FileCache[pinfo.IP])
				}
			}
		}
	}()
}