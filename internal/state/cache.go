package state

import (
	"sync"
	"time"
)

// FileInfo reducido para evitar importar fs
type FileInfo struct {
	Name    string
	ModTime time.Time
}

var FileCache = make(map[string][]FileInfo)
var OnlineStatus = make(map[string]bool)

// Elimina un archivo del cache por IP y nombre
func RemoveFileFromCache(ip, filename string) {
	list := FileCache[ip]
	newList := []FileInfo{}
	for _, f := range list {
		if f.Name != filename {
			newList = append(newList, f)
		}
	}
	FileCache[ip] = newList
}

// ===============================
// Operaciones pendientes por nodo
// ===============================

type PendingOperation struct {
	Type     string // "send", "get", "delete"
	FilePath string
	TargetID int    // Nodo al que va dirigida la operación
	SourceID int    // Nodo que originó la operación
}

var (
	pendingOps = make(map[int][]PendingOperation) // clave es int
	mutex      sync.Mutex
)

func AddPendingOp(peerID int, op PendingOperation) {
	mutex.Lock()
	defer mutex.Unlock()
	pendingOps[peerID] = append(pendingOps[peerID], op)
}

func GetAndClearPendingOps(peerID int) []PendingOperation {
	mutex.Lock()
	defer mutex.Unlock()
	ops := pendingOps[peerID]
	delete(pendingOps, peerID)
	return ops
}

func PeekPendingOps(peerID int) []PendingOperation {
	mutex.Lock()
	defer mutex.Unlock()
	return pendingOps[peerID]
}
