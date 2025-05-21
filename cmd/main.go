package main

import (
	"p2pfs/internal/peer"
	"p2pfs/internal/gui"
)

func main() {
	peerSystem := peer.InitPeer()
	if peerSystem == nil {
		return
	}

	go peer.StartServer(peerSystem.Local.Port)
	gui.Run(peerSystem)
}
