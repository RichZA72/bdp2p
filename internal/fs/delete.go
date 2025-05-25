package fs

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

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
		if info.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			return fmt.Errorf("error al eliminar localmente: %w", err)
		}
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

	// 🌐 Nodo conectado → enviar solicitud de eliminación (archivo o carpeta)
	go func() {
		if isDirPath(selected.FileName) {
			sendDeleteDirRequest(*remotePeer, selected.FileName)
		} else {
			sendDeleteRequest(*remotePeer, selected.FileName)
		}
		peer.SendSyncLog("DELETE", selected.FileName, localID, remotePeer.ID)
	}()
	return nil
}

// isDirPath intenta inferir si es carpeta (heurística simple)
func isDirPath(name string) bool {
	return !strings.Contains(filepath.Base(name), ".")
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

// sendDeleteDirRequest envía un mensaje DELETE_DIR a un nodo remoto por TCP
func sendDeleteDirRequest(p peer.PeerInfo, dirname string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		fmt.Println("❌ No se pudo conectar para eliminar carpeta:", err)
		return
	}
	defer conn.Close()

	msg := map[string]string{
		"type": "DELETE_DIR",
		"name": dirname,
	}
	json.NewEncoder(conn).Encode(msg)
}
