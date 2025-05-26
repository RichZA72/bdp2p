package fs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"p2pfs/internal/peer"
	"p2pfs/internal/state"
)

// SendFileToPeer envía un archivo o carpeta local a otro nodo
func SendFileToPeer(p peer.PeerInfo, filename string) error {
	cleanPath := filepath.Clean(filename)
	filePath := filepath.Join("shared", cleanPath)

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("no se pudo acceder a %s: %w", cleanPath, err)
	}

	fmt.Printf("📦 Enviando %s — Es directorio: %v\n", cleanPath, info.IsDir())

	if info.IsDir() {
		return sendDirectoryRecursively(p, cleanPath)
	}

	return sendSingleFile(p, filePath, filepath.Base(cleanPath))
}

// sendSingleFile envía un archivo sin su ruta original
func sendSingleFile(p peer.PeerInfo, fullPath string, sendAsName string) error {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", fullPath, err)
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", p.IP, p.Port))
	if err != nil {
		return fmt.Errorf("no se pudo conectar a %s: %w", p.IP, err)
	}
	defer conn.Close()

	msg := map[string]interface{}{
		"type":    "SEND_FILE",
		"name":    sendAsName,
		"content": base64.StdEncoding.EncodeToString(data),
		"isDir":   false,
	}
	return json.NewEncoder(conn).Encode(msg)
}

// sendDirectoryRecursively envía todos los archivos dentro de una carpeta con estructura
func sendDirectoryRecursively(p peer.PeerInfo, root string) error {
	rootPath := filepath.Join("shared", root)

	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Saltar la carpeta contenedora, no registrar en FileCache
		if info.IsDir() {
			return nil
		}

		// Obtener ruta relativa (ej: dir1/a.txt)
		relPath, _ := filepath.Rel("shared", path)

		// Si el nodo está desconectado → registrar como pendiente
		if !state.OnlineStatus[p.IP] {
			state.FileCache[p.IP] = append(state.FileCache[p.IP], state.FileInfo{
				Name:    relPath,
				ModTime: info.ModTime(),
				IsDir:   false,
			})
			state.AddPendingOp(p.ID, state.PendingOperation{
				Type:     "send",
				FilePath: relPath,
				TargetID: p.ID,
				SourceID: peer.Local.ID,
			})
			peer.SendSyncLog("TRANSFER", relPath, peer.Local.ID, p.ID)
			fmt.Printf("📦 Pendiente: %s para %s\n", relPath, p.IP)
			return nil
		}

		// Nodo en línea → enviar inmediatamente
		return sendSingleFile(p, path, relPath)
	})
}



// RequestFileFromPeer solicita un archivo desde otro nodo
func RequestFileFromPeer(p peer.PeerInfo, filename string, flatten bool) error {
	if !state.OnlineStatus[p.IP] {
		state.AddPendingOp(p.ID, state.PendingOperation{
			Type:     "get",
			FilePath: filename,
			TargetID: peer.Local.ID,
			SourceID: p.ID,
			Flatten:  flatten, // ✅ nuevo campo
		})
		peer.SendSyncLog("GET_FILE", filename, p.ID, peer.Local.ID)
		fmt.Printf("📥 Solicitud pendiente: archivo '%s' será enviado desde %s al reconectarse\n", filename, p.IP)
		return nil
	}

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
		errMsg, _ := resp["error"].(string)
		return fmt.Errorf("respuesta inesperada del peer: %v", errMsg)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp["content"].(string))
	if err != nil {
		return fmt.Errorf("error al decodificar contenido: %w", err)
	}

	// ✅ Cambiar forma de guardar según flatten
	var path string
	if flatten {
		path = filepath.Join("shared", filepath.Base(filename)) // sin carpeta
	} else {
		path = filepath.Join("shared", filename) // con estructura
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("error creando carpetas destino: %w", err)
	}

	if err := os.WriteFile(path, decoded, 0644); err != nil {
		return fmt.Errorf("error al guardar archivo: %w", err)
	}

	fmt.Println("✅ Archivo transferido desde", p.IP, "→", path)
	return nil
}



// RequestDirectoryFromPeer solicita todos los archivos dentro de un directorio remoto
func RequestDirectoryFromPeer(p peer.PeerInfo, dir string) error {
	if !state.OnlineStatus[p.IP] {
		fmt.Printf("📥 Nodo %s desconectado, registrando solicitud de carpeta %s como pendiente\n", p.IP, dir)
		
		// Obtener archivos del FileCache de la última sincronización
		for _, f := range state.FileCache[p.IP] {
			if strings.HasPrefix(f.Name, dir+"/") && !f.IsDir {
				state.AddPendingOp(p.ID, state.PendingOperation{
					Type:     "get",
					FilePath: f.Name,
					TargetID: peer.Local.ID,
					SourceID: p.ID,
				})
				// Mostrar visualmente lo que llegará
				state.FileCache[p.IP] = append(state.FileCache[p.IP], state.FileInfo{
					Name:    f.Name,
					ModTime: f.ModTime,
					IsDir:   false,
				})
				peer.SendSyncLog("GET_FILE", f.Name, p.ID, peer.Local.ID)
			}
		}
		return nil
	}

	// Nodo en línea → obtener lista remota y solicitar cada archivo
	files, err := requestRemoteFileList(p, dir)
	if err != nil {
		return fmt.Errorf("no se pudo obtener archivos de %s: %w", dir, err)
	}

	if len(files) == 0 {
		return fmt.Errorf("el directorio remoto está vacío o no se encontró: %s", dir)
	}

	for _, f := range files {
		err := RequestFileFromPeer(p, f.Name, false)
		if err != nil {
			fmt.Printf("⚠️ Error al solicitar %s: %v\n", f.Name, err)
		}
	}

	return nil
}


// RelayFileBetweenPeers reenvía un archivo o carpeta desde un nodo fuente a múltiples destinos
func RelayFileBetweenPeers(source peer.PeerInfo, filename string, targets []peer.PeerInfo) error {
	filename = filepath.Clean(filename)

	files, err := requestRemoteFileList(source, filename)
	if err != nil {
		return fmt.Errorf("no se pudo obtener lista de archivos de %s: %w", filename, err)
	}
	if len(files) > 0 {
		for _, f := range files {
			rel := f.Name
			RelayFileBetweenPeers(source, rel, targets)
		}
		return nil
	}

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
	// Registrar como pendiente cada archivo del directorio
	if len(files) > 0 {
		for _, f := range files {
			state.FileCache[target.IP] = append(state.FileCache[target.IP], state.FileInfo{
				Name:    f.Name,
				ModTime: f.ModTime,
				IsDir:   f.IsDir,
			})
			state.AddPendingOp(target.ID, state.PendingOperation{
				Type:     "send",
				FilePath: f.Name,
				TargetID: target.ID,
				SourceID: source.ID,
			})
			peer.SendSyncLog("TRANSFER", f.Name, source.ID, target.ID)
		}
	} else {
		// Caso: archivo único
		state.FileCache[target.IP] = append(state.FileCache[target.IP], state.FileInfo{
			Name:    filename,
			ModTime: time.Now(),
			IsDir:   false,
		})
		state.AddPendingOp(target.ID, state.PendingOperation{
			Type:     "send",
			FilePath: filename,
			TargetID: target.ID,
			SourceID: source.ID,
		})
		peer.SendSyncLog("TRANSFER", filename, source.ID, target.ID)
		}
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
			"isDir":   false,
		}
		json.NewEncoder(connT).Encode(msg)
		peer.SendSyncLog("TRANSFER", filename, source.ID, target.ID)
	}

	return nil
}

// requestRemoteFileList obtiene lista recursiva de archivos desde un nodo remoto
func requestRemoteFileList(peer peer.PeerInfo, dir string) ([]state.FileInfo, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := map[string]interface{}{
		"type": "GET_FILES",
	}
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

	// ✅ Fix aquí: para detectar todos los archivos dentro del directorio solicitado
	dir = strings.TrimSuffix(dir, "/") // aseguramos que no tenga / al final

	var result []state.FileInfo
	for _, f := range files {
		if f.Name == dir || strings.HasPrefix(f.Name, dir+"/") {
			result = append(result, f)
		}
	}
	return result, nil
}




// TransferFile decide cómo enviar un archivo o carpeta basado en el origen y destinos seleccionados
func TransferFile(peerSystem *peer.Peer, selected SelectedFile, checkedPeers map[int]bool) (int, error) {
	localID := peerSystem.Local.ID
	count := 0

	if selected.PeerID != localID && !anyChecked(checkedPeers) {
	for _, p := range peerSystem.Peers {
		if p.ID == selected.PeerID {
			// Buscar si el archivo seleccionado es un directorio usando FileCache
			for _, f := range state.FileCache[p.IP] {
				if f.Name == selected.FileName && f.IsDir {
					return 1, RequestDirectoryFromPeer(p, selected.FileName)
					}
				}
				return 1, RequestFileFromPeer(p, selected.FileName, true)
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
					err := SendFileToPeer(p, selected.FileName)
					if err != nil {
						path := filepath.Join("shared", selected.FileName)
						info, err := os.Stat(path)
						isDir := false
						if err == nil {
							isDir = info.IsDir()
						}

						state.FileCache[p.IP] = append(state.FileCache[p.IP], state.FileInfo{
							Name:    selected.FileName,
							ModTime: time.Now(),
							IsDir:   isDir,
						})

						state.AddPendingOp(p.ID, state.PendingOperation{
							Type:     "send",
							FilePath: selected.FileName,
							TargetID: p.ID,
							SourceID: localID,
						})
						peer.SendSyncLog("TRANSFER", selected.FileName, localID, p.ID)
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
		return len(targets), RelayFileBetweenPeers(source, selected.FileName, targets)
	}

	return 0, fmt.Errorf("ninguna operación válida de transferencia")
}

func anyChecked(m map[int]bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}
