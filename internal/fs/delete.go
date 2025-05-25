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
		// 🏠 Eliminación local (archivo o carpeta)
		path := filepath.Join("shared", selected.FileName)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("no se encontró el archivo o carpeta: %w", err)
		}
		var delErr error
		if info.IsDir() {
			delErr = os.RemoveAll(path)
		} else {
			delErr = os.Remove(path)
		}
		if delErr != nil {
			return fmt.Errorf("error al eliminar localmente: %w", delErr)
		}
		peer.SendSyncLog("DELETE", selected.FileName, localID, localID)
		return nil
	}

	// 🌐 Eliminación remota o diferida
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
		// Nodo desconectado → eliminar visualmente y registrar como pendiente
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

	// Nodo conectado → enviar solicitud de eliminación
	go func() {
		sendDeleteRequest(*remotePeer, selected.FileName)
		peer.SendSyncLog("DELETE", selected.FileName, localID, remotePeer.ID)
	}()
	return nil
}

// sendDeleteRequest envía DELETE_FILE a un nodo remoto, que debe decidir si es archivo o carpeta
func sendDeleteRequest(p peer.PeerInfo, path string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		fmt.Println("❌ No se pudo conectar para eliminar:", err)
		return
	}
	defer conn.Close()

	msg := map[string]string{
		"type": "DELETE_FILE",
		"name": path,
	}
	json.NewEncoder(conn).Encode(msg)
}
