package peer

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "os"
    "path/filepath"
)

type PeerInfo struct {
    ID    int    `json:"id"`
    IP    string `json:"ip"`
    Port  string `json:"port"`
    Local bool   `json:"local"`
}

var LocalPeer PeerInfo
var Peers []PeerInfo

func Start() {
    err := loadPeers()
    if err != nil {
        fmt.Println("Error al cargar peers.json:", err)
        return
    }

    fmt.Printf("Nodo local: ID=%d IP=%s Puerto=%s\n", LocalPeer.ID, LocalPeer.IP, LocalPeer.Port)

    go startTCPServer(LocalPeer.Port)

    // Aquí puedes iniciar otros procesos, como sincronización, etc.
}

func loadPeers() error {
    data, err := os.ReadFile("config/peers.json")
    if err != nil {
        return err
    }

    err = json.Unmarshal(data, &Peers)
    if err != nil {
        return err
    }

    for _, p := range Peers {
        if p.Local {
            LocalPeer = p
            return nil
        }
    }

    return fmt.Errorf("No se encontró un nodo con 'local: true' en peers.json")
}

func startTCPServer(port string) {
    ln, err := net.Listen("tcp", ":"+port)
    if err != nil {
        fmt.Println("Error al iniciar servidor TCP:", err)
        return
    }

    fmt.Println("Servidor TCP escuchando en puerto", port)

    for {
        conn, err := ln.Accept()
        if err != nil {
            fmt.Println("Error al aceptar conexión:", err)
            continue
        }
        go handleIncomingFile(conn)
    }
}

func handleIncomingFile(conn net.Conn) {
    defer conn.Close()

    reader := bufio.NewReader(conn)

    // Leer la primera línea: nombre del archivo
    fileName, err := reader.ReadString('\n')
    if err != nil {
        fmt.Println("Error al leer nombre de archivo:", err)
        return
    }
    fileName = filepath.Base(fileName[:len(fileName)-1]) // quitar \n y proteger ruta

    // Crear archivo en carpeta shared
    os.MkdirAll("./shared", os.ModePerm)
    f, err := os.Create("./shared/" + fileName)
    if err != nil {
        fmt.Println("Error al crear archivo:", err)
        return
    }
    defer f.Close()

    // Copiar el resto del contenido
    _, err = io.Copy(f, reader)
    if err != nil {
        fmt.Println("Error al guardar contenido:", err)
        return
    }

    fmt.Printf("✅ Archivo '%s' recibido y guardado correctamente.\n", fileName)
}
