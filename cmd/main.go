package main

import (
	"p2pfs/internal/peer"
	"p2pfs/internal/gui"
)

func main() {
	go peer.Start()  // Servidor TCP en segundo plano
	gui.Run()        // GUI bloquea y permanece abierta
}
