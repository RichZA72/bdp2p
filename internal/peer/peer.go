package peer

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"p2pfs/internal/state"
)

type PeerInfo struct {
	ID      int    `json:"id"`
	IP      string `json:"ip"`
	Port    string `json:"port"`
	IsLocal bool   `json:"is_local"`
}

type Peer struct {
	Local PeerInfo
	Peers []PeerInfo
}

type PeerStatus struct {
	Peer   PeerInfo
	Online bool
}

var instanciaGlobal *Peer

func LoadPeers(configPath string) (*Peer, error) {
	fmt.Println("📄 Leyendo archivo:", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el archivo de configuración: %v", err)
	}
	defer file.Close()

	var peers []PeerInfo
	if err := json.NewDecoder(file).Decode(&peers); err != nil {
		return nil, fmt.Errorf("error al decodificar JSON: %v", err)
	}

	fmt.Println("✅ Peers cargados desde JSON:")
	for _, p := range peers {
		fmt.Printf("- ID: %d | IP: %s | Port: %s | is_local: %v\n", p.ID, p.IP, p.Port, p.IsLocal)
	}

	var local PeerInfo
	found := false
	for _, p := range peers {
		if p.IsLocal {
			local = p
			found = true
			break
		}
	}

	if !found {
		fmt.Println("❌ No se encontró un nodo con 'is_local': true en peers.json")
		return nil, nil
	}

	peer := &Peer{
		Local: local,
		Peers: peers,
	}
	instanciaGlobal = peer
	return peer, nil
}

func InitPeer() *Peer {
	configPath := filepath.Join("config", "peers.json")
	peer, err := LoadPeers(configPath)
	if err != nil || peer == nil {
		return nil
	}
	fmt.Printf("🟢 Nodo local detectado: ID %d, IP %s, Puerto %s\n", peer.Local.ID, peer.Local.IP, peer.Local.Port)
	return peer
}

func IsPeerOnline(p PeerInfo) bool {
	address := fmt.Sprintf("%s:%s", p.IP, p.Port)
	conn, err := net.DialTimeout("tcp", address, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (p *Peer) GetPeerStatuses() []PeerStatus {
	var statuses []PeerStatus
	for _, peer := range p.Peers {
		if peer.ID == p.Local.ID {
			continue
		}
		s := PeerStatus{
			Peer:   peer,
			Online: IsPeerOnline(peer),
		}
		statuses = append(statuses, s)
	}
	return statuses
}

func Start() {
	p := InitPeer()
	if p == nil {
		fmt.Println("⚠️ Abortando Start: nodo local no detectado.")
		return
	}
	go StartServer(p.Local.Port)
}

func ActualizarEstadoDeNodo(p Peer) {
	state.OnlineStatus[p.Local.IP] = IsPeerOnline(p.Local)
	// Este método puede ser removido o reescrito si ya no es necesario
}

func GetLocalIP() string {
	if instanciaGlobal != nil {
		return instanciaGlobal.Local.IP
	}
	return ""
}

func GetPeers() []PeerInfo {
	if instanciaGlobal != nil {
		return instanciaGlobal.Peers
	}
	return nil
}
