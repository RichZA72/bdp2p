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

// TransferFile realiza la lógica general de transferencia según el origen y destino
func TransferFile(peerSystem *peer.Peer, selected SelectedFile, checkedPeers map[int]bool) (int, error) {
	localID := peerSystem.Local.ID
	count := 0

	// Caso 1: archivo remoto → traerlo localmente si no hay destino marcado
	if selected.PeerID != localID && !anyChecked(checkedPeers) {
		for _, p := range peerSystem.Peers {
			if p.ID == selected.PeerID {
				return 1, RequestFileFromPeer(p, selected.FileName)
			}
		}
		return 0, fmt.Errorf("peer origen no encontrado")
	}

	// Caso 2 y 3: enviar archivo (local o remoto) a otros nodos
	for targetID, checked := range checkedPeers {
		if !checked {
			continue
		}
		for _, p := range peerSystem.Peers {
			if p.ID == targetID {
				if !state.OnlineStatus[p.IP] {
					// Guardar visualmente en caché si está desconectado
					state.FileCache[p.IP] = append(state.FileCache[p.IP], state.FileInfo{
						Name:    selected.FileName,
						ModTime: time.Now(),
					})
					count++
					continue
				}
				// Enviar archivo
				err := SendFileToPeer(p, selected.FileName)
				if err != nil {
					fmt.Printf("❌ Error al enviar a %s: %v\n", p.IP, err)
				}
				count++
			}
		}
	}

	if count == 0 {
		return 0, fmt.Errorf("no se seleccionó ninguna máquina")
	}
	return count, nil
}

// anyChecked evalúa si alguna máquina fue seleccionada
func anyChecked(m map[int]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}
