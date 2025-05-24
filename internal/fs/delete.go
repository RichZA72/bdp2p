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

// DeleteFile maneja eliminación local, remota o desconectada
func DeleteFile(peerSystem *peer.Peer, selected SelectedFile) error {
	localID := peerSystem.Local.ID

	if selected.PeerID == localID {
		// 🏠 Eliminación local
		path := filepath.Join("shared", selected.FileName)
		err := os.Remove(path)
		if err != nil {
			return fmt.Errorf("error al eliminar archivo local: %w", err)
		}
		return nil
	}

	// Buscar al peer destino
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
		// ❌ Nodo desconectado → eliminación visual + registrar pendiente
		state.RemoveFileFromCache(remotePeer.IP, selected.FileName)
		state.AddPendingOp(remotePeer.ID, state.PendingOperation{
			Type:     "delete",
			FilePath: selected.FileName,
			TargetID: remotePeer.ID,
			SourceID: peerSystem.Local.ID,
		})
		// 🔄 Notificar visualmente a los demás
		peer.SendSyncLog("DELETE", selected.FileName, peerSystem.Local.ID, remotePeer.ID)
		return fmt.Errorf("nodo desconectado, archivo eliminado visualmente y registrado")
	}

	// 🌐 Nodo en línea → eliminación remota
	go func() {
		sendDeleteRequest(*remotePeer, selected.FileName)
		peer.SendSyncLog("DELETE", selected.FileName, peerSystem.Local.ID, remotePeer.ID)
	}()
	return nil
}

// sendDeleteRequest envía un mensaje DELETE_FILE a otro nodo
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
