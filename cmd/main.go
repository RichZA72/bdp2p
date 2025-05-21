package main

import (

    "p2pfs/internal/peer"
    "p2pfs/internal/gui"
)


func main() {
	    go peer.Start() // Servidor en segundo plano
    	    gui.Run()       // Iniciar GUI en primer plano
}
