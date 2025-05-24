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

// SendFileToPeer envía un archivo local a otro nodo
func SendFileToPeer(p peer.PeerInfo, filename string) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		return fmt.Errorf("no se pudo conectar a %s: %w", p.IP, err)
	}
	defer conn.Close()

	data, err := os.ReadFile(filepath.Join("shared", filename))
	if err != nil {
		return fmt.Errorf("no se pudo leer el archivo: %w", err)
	}

	msg := map[string]interface{}{
		"type":    "SEND_FILE",
		"name":    filename,
		"content": base64.StdEncoding.EncodeToString(data),
	}
	return json.NewEncoder(conn).Encode(msg)
}

// RequestFileFromPeer solicita un archivo desde otro nodo y lo guarda localmente
func RequestFileFromPeer(p peer.PeerInfo, filename string) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		return fmt.Errorf("no se pudo conectar a %s: %w", p.IP, err)
	}
	defer conn.Close()

	req := map[string]interface{}{
		"type": "GET_FILE",
		"name": filename,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("no se pudo enviar la solicitud: %w", err)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("error al recibir archivo: %w", err)
	}
	if resp["type"] != "FILE_CONTENT" {
		return fmt.Errorf("respuesta inesperada del peer")
	}

	decoded, err := base64.StdEncoding.DecodeString(resp["content"].(string))
	if err != nil {
		return fmt.Errorf("error al decodificar contenido: %w", err)
	}

	return os.WriteFile(filepath.Join("shared", filename), decoded, 0644)
}

// RelayFileBetweenPeers solicita un archivo desde un nodo remoto y lo reenvía a otros nodos destino
func RelayFileBetweenPeers(source peer.PeerInfo, filename string, targets []peer.PeerInfo) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", source.IP, source.Port))
	if err != nil {
		return fmt.Errorf("no se pudo conectar al peer fuente %s: %w", source.IP, err)
	}
	defer conn.Close()

	req := map[string]interface{}{
		"type": "GET_FILE",
		"name": filename,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("no se pudo enviar solicitud: %w", err)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("error al recibir archivo: %w", err)
	}
	if resp["type"] != "FILE_CONTENT" {
		return fmt.Errorf("respuesta inesperada del peer")
	}

	encoded, ok := resp["content"].(string)
	if !ok {
		return fmt.Errorf("contenido inválido")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("error al decodificar contenido: %w", err)
	}

	for _, target := range targets {
		if !state.OnlineStatus[target.IP] {
			state.FileCache[target.IP] = append(state.FileCache[target.IP], state.FileInfo{
				Name:    filename,
				ModTime: time.Now(),
			})
			state.AddPendingOp(target.ID, state.PendingOperation{
				Type:     "send",
				FilePath: filename,
				TargetID: target.ID,
				SourceID: source.ID,
			})
			peer.SendSyncLog("TRANSFER", filename, source.ID, target.ID)
			continue
		}

		connT, err := net.Dial("tcp", fmt.Sprintf("%s:%s", target.IP, target.Port))
		if err != nil {
			state.FileCache[target.IP] = append(state.FileCache[target.IP], state.FileInfo{
				Name:    filename,
				ModTime: time.Now(),
			})
			state.AddPendingOp(target.ID, state.PendingOperation{
				Type:     "send",
				FilePath: filename,
				TargetID: target.ID,
				SourceID: source.ID,
			})
			peer.SendSyncLog("TRANSFER", filename, source.ID, target.ID)
			continue
		}
		defer connT.Close()

		msg := map[string]interface{}{
			"type":    "SEND_FILE",
			"name":    filename,
			"content": base64.StdEncoding.EncodeToString(data),
		}
		json.NewEncoder(connT).Encode(msg)
		peer.SendSyncLog("TRANSFER", filename, source.ID, target.ID)
	}

	return nil
}

// TransferFile realiza la lógica general de transferencia según el origen y destino
func TransferFile(peerSystem *peer.Peer, selected SelectedFile, checkedPeers map[int]bool) (int, error) {
	localID := peerSystem.Local.ID
	count := 0

	if selected.PeerID != localID && !anyChecked(checkedPeers) {
		for _, p := range peerSystem.Peers {
			if p.ID == selected.PeerID {
				return 1, RequestFileFromPeer(p, selected.FileName)
			}
		}
		return 0, fmt.Errorf("peer origen no encontrado")
	}

	if selected.PeerID == localID {
		for targetID, checked := range checkedPeers {
			if !checked {
				continue
			}
			for _, p := range peerSystem.Peers {
				if p.ID == targetID {
					if !state.OnlineStatus[p.IP] {
						state.FileCache[p.IP] = append(state.FileCache[p.IP], state.FileInfo{
							Name:    selected.FileName,
							ModTime: time.Now(),
						})
						state.AddPendingOp(p.ID, state.PendingOperation{
							Type:     "send",
							FilePath: selected.FileName,
							TargetID: p.ID,
							SourceID: peerSystem.Local.ID,
						})
						peer.SendSyncLog("TRANSFER", selected.FileName, localID, p.ID)
						count++
						continue
					}
					err := SendFileToPeer(p, selected.FileName)
					if err != nil {
						fmt.Printf("❌ Error al enviar a %s: %v\n", p.IP, err)
					} else {
						peer.SendSyncLog("TRANSFER", selected.FileName, localID, p.ID)
					}
					count++
				}
			}
		}
		return count, nil
	}

	if selected.PeerID != localID && anyChecked(checkedPeers) {
		var source peer.PeerInfo
		var targets []peer.PeerInfo
		for _, p := range peerSystem.Peers {
			if p.ID == selected.PeerID {
				source = p
			} else if checkedPeers[p.ID] {
				targets = append(targets, p)
			}
		}

		if !state.OnlineStatus[source.IP] {
			for _, target := range targets {
				state.FileCache[target.IP] = append(state.FileCache[target.IP], state.FileInfo{
					Name:    selected.FileName,
					ModTime: time.Now(),
				})
				state.AddPendingOp(source.ID, state.PendingOperation{
					Type:     "get",
					FilePath: selected.FileName,
					TargetID: target.ID,
					SourceID: target.ID,
				})
				peer.SendSyncLog("TRANSFER", selected.FileName, source.ID, target.ID)
				fmt.Printf("📩 Archivo '%s' solicitado a %s → será enviado a %s cuando se reconecte\n",
					selected.FileName, source.IP, target.IP)
			}
			return len(targets), nil
		}

		err := RelayFileBetweenPeers(source, selected.FileName, targets)
		if err != nil {
			return 0, err
		}
		return len(targets), nil
	}

	return 0, fmt.Errorf("ninguna operación de transferencia válida")
}

func anyChecked(m map[int]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}
