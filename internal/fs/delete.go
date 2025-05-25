package fs

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"p2pfs/internal/peer"
	"p2pfs/internal/state"
)

// DeleteFile maneja eliminación local, remota o diferida (pendiente)
func DeleteFile(peerSystem *peer.Peer, selected SelectedFile) error {
	localID := peerSystem.Local.ID

	if selected.PeerID == localID {
		// 🏠 Eliminación local
		path := filepath.Join("shared", selected.FileName)
		err := os.Remove(path)
		if err != nil {
			return fmt.Errorf("error al eliminar archivo local: %w", err)
		}
		// 🔄 Notificar a los otros nodos para reflejar el cambio
		peer.SendSyncLog("DELETE", selected.FileName, localID, localID)
		return nil
	}

	// Buscar información del nodo remoto
	var remotePeer *peer.PeerInfo
	for _, p := range peerSystem.Peers {
		if p.ID == selected.PeerID {
			remotePeer = &p
			break
		}
	}
	if remotePeer == nil {
		return fmt.Errorf("peer no encontrado")
	}

	if !state.OnlineStatus[remotePeer.IP] {
		// ❌ Nodo desconectado → eliminación visual y registro como pendiente
		state.RemoveFileFromCache(remotePeer.IP, selected.FileName)
		state.AddPendingOp(remotePeer.ID, state.PendingOperation{
			Type:     "delete",
			FilePath: selected.FileName,
			TargetID: remotePeer.ID,
			SourceID: localID,
		})
		peer.SendSyncLog("DELETE", selected.FileName, localID, remotePeer.ID)
		return fmt.Errorf("nodo desconectado, eliminación registrada como pendiente")
	}

	// 🌐 Nodo conectado → enviar solicitud de eliminación
	go func() {
		sendDeleteRequest(*remotePeer, selected.FileName)
		peer.SendSyncLog("DELETE", selected.FileName, localID, remotePeer.ID)
	}()
	return nil
}

// sendDeleteRequest envía un mensaje DELETE_FILE a un nodo remoto por TCP
func sendDeleteRequest(p peer.PeerInfo, filename string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		fmt.Println("❌ No se pudo conectar para eliminar archivo:", err)
		return
	}
	defer conn.Close()

	msg := map[string]string{
		"type": "DELETE_FILE",
		"name": filename,
	}
	json.NewEncoder(conn).Encode(msg)
}
