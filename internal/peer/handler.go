package peer

import (
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net"
    "os"
    "path/filepath"
    "time"

    )


// Estas dos variables deben ser inicializadas desde fuera
var Local PeerInfo
var Peers []PeerInfo

type FileInfo struct {
    Name    string    `json:"name"`
    ModTime time.Time `json:"modTime"`
}

// StartServer inicia el servidor TCP en el puerto local
func StartServer(port string) {
    listener, err := net.Listen("tcp", ":"+port)
    if err != nil {
        fmt.Println("❌ Error iniciando servidor:", err)
        return
    }
    fmt.Println("🟢 Servidor TCP escuchando en el puerto", port)

    for {
        conn, err := listener.Accept()
        if err != nil {
            fmt.Println("⚠️ Error al aceptar conexión:", err)
            continue
        }
        go handleConnection(conn)
    }
}

// handleConnection maneja solicitudes entrantes
func handleConnection(conn net.Conn) {
    defer conn.Close()

    var request map[string]interface{}
    err := json.NewDecoder(conn).Decode(&request)
    if err != nil {
        fmt.Println("⚠️ Error al decodificar mensaje:", err)
        return
    }

    switch request["type"] {
    case "GET_FILES":
        handleGetFiles(conn)

    case "GET_FILE":
        name := request["name"].(string)
        handleSendFile(conn, name)

    case "SEND_FILE":
        handleReceiveFile(request)

    case "DELETE_FILE":
        name := request["name"].(string)
        handleDeleteFile(conn, name)

    case "SYNC_LOGS":
        handleSyncLogs(request)
    }
}

// --- HANDLERS ---

func handleGetFiles(conn net.Conn) {
    files, err := getLocalFiles()
    if err != nil {
        return
    }
    resp := map[string]interface{}{
        "type":  "FILES_LIST",
        "files": files,
    }
    json.NewEncoder(conn).Encode(resp)
}

func handleSendFile(conn net.Conn, name string) {
    path := filepath.Join("shared", name)
    data, err := os.ReadFile(path)
    if err != nil {
        fmt.Println("❌ No se pudo leer el archivo:", err)
        return
    }

    resp := map[string]interface{}{
        "type":    "FILE_CONTENT",
        "name":    name,
        "content": base64.StdEncoding.EncodeToString(data),
    }
    json.NewEncoder(conn).Encode(resp)
}

func handleReceiveFile(request map[string]interface{}) {
    name := request["name"].(string)
    encoded := request["content"].(string)

    data, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        fmt.Println("❌ Error al decodificar archivo:", err)
        return
    }

    err = os.WriteFile(filepath.Join("shared", name), data, 0644)
    if err != nil {
        fmt.Println("❌ Error al guardar archivo recibido:", err)
        return
    }

    fmt.Println("📥 Archivo recibido y guardado:", name)
}

func handleDeleteFile(conn net.Conn, name string) {
    err := os.Remove(filepath.Join("shared", name))
    status := "ok"
    if err != nil {
        fmt.Println("❌ Error al eliminar archivo:", err)
        status = "error"
    }
    resp := map[string]interface{}{
        "type":   "DELETE_ACK",
        "status": status,
    }
    json.NewEncoder(conn).Encode(resp)
}

// getLocalFiles devuelve info de los archivos locales
func getLocalFiles() ([]FileInfo, error) {
    var files []FileInfo
    dir := "shared"

    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }

    for _, entry := range entries {
        if !entry.IsDir() {
            info, err := entry.Info()
            if err != nil {
                continue
            }
            files = append(files, FileInfo{
                Name:    entry.Name(),
                ModTime: info.ModTime(),
            })
        }
    }
    return files, nil
}

// handleSyncLogs aplica los logs recibidos si son dirigidos a esta máquina
func handleSyncLogs(request map[string]interface{}) {
    logsData, ok := request["logs"].([]interface{})
    if !ok {
        fmt.Println("❌ Formato inválido de logs.")
        return
    }

    for _, item := range logsData {
        entryMap, ok := item.(map[string]interface{})
        if !ok {
            continue
        }

        action := entryMap["action"].(string)
        fileName := entryMap["fileName"].(string)
        originID := int(entryMap["originID"].(float64))
        targetID := int(entryMap["targetID"].(float64))

        // Solo aplicar si es para esta máquina
        if targetID != Local.ID {
            continue
        }

        switch action {
        case "DELETE":
            err := os.Remove(filepath.Join("shared", fileName))
            if err == nil {
                fmt.Println("🗑️ Archivo eliminado por log:", fileName)
            } else {
                fmt.Println("⚠️ No se pudo eliminar:", fileName)
            }

        case "TRANSFER":
            for _, peer := range Peers {
                if peer.ID == originID {
                    go requestFileFromPeer(peer, fileName)
                    break
                }
            }
        }
    }
}

// Solicita un archivo a otro nodo
func requestFileFromPeer(peer PeerInfo, filename string) {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
    if err != nil {
        fmt.Println("❌ No se pudo conectar a", peer.IP)
        return
    }
    defer conn.Close()

    req := map[string]interface{}{
        "type": "GET_FILE",
        "name": filename,
    }
    json.NewEncoder(conn).Encode(req)

    var resp map[string]interface{}
    err = json.NewDecoder(conn).Decode(&resp)
    if err != nil || resp["type"] != "FILE_CONTENT" {
        fmt.Println("❌ Error al recibir archivo:", err)
        return
    }

    decoded, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
    os.WriteFile(filepath.Join("shared", filename), decoded, 0644)
}
