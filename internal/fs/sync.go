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
)

func ResyncAfterReconnect(localFiles []FileInfo) {
    myIP := peer.GetLocalIP()

    fmt.Println("🔄 Ejecutando ResyncAfterReconnect...")

    for _, p := range peer.GetPeers() {
        if p.IP == myIP {
            continue
        }

        remoteFiles, err := requestFileListFromPeer(p.IP)
        if err != nil {
            fmt.Println("❌ No se pudo obtener archivos de", p.IP)
            continue
        }

        for _, f := range remoteFiles {
            if !fileExistsLocally(f.Name, localFiles) {
                fmt.Println("📥 Solicitando archivo que nos transfirieron:", f.Name)
                requestFileFromPeer(p.IP, f.Name)
            }
        }

	for _, f := range localFiles {
	    if !fileInList(f.Name, remoteFiles) {
	        ruta := filepath.Join("shared", f.Name) // ✅ Corrección aquí
	        fmt.Println("🗑️ Eliminando archivo que fue eliminado remotamente:", ruta)
	        os.Remove(ruta)
	    }
	}

    }

    fmt.Println("✅ ResyncAfterReconnect finalizado.")
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

    req := map[string]interface{}{ "type": "GET_FILE", "name": filename }
    json.NewEncoder(conn).Encode(req)

    var resp map[string]interface{}
    if err := json.NewDecoder(conn).Decode(&resp); err != nil || resp["type"] != "FILE_CONTENT" {
        fmt.Println("❌ Error al recibir archivo:", err)
        return
    }

    data, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
    os.WriteFile(filepath.Join("shared", filename), data, 0644)
}
