package state

import (
	"sync"
	"time"
)

// ===============================
// Información de archivos por nodo
// ===============================

// FileInfo es una versión reducida para evitar importar fs
type FileInfo struct {
	Name    string
	ModTime time.Time
	IsDir   bool // ← nuevo campo para indicar si es carpeta
}

// FileCache guarda la lista de archivos por IP de nodo
var FileCache = make(map[string][]FileInfo)

// OnlineStatus indica si un nodo está en línea por su IP
var OnlineStatus = make(map[string]bool)

// RemoveFileFromCache elimina un archivo del cache por IP y nombre de archivo
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

// PendingOperation representa una operación diferida hacia un nodo
type PendingOperation struct {
	Type     string // "send", "get", "delete"
	FilePath string
	TargetID int // Nodo destinatario
	SourceID int // Nodo origen (quien inicia la operación)
	// Futuras extensiones: Estado visual, timestamp, etc.
}

var (
	pendingOps = make(map[int][]PendingOperation) // Mapa de operaciones pendientes por ID de nodo
	mutex      sync.Mutex
)

// AddPendingOp agrega una operación pendiente para un nodo dado
func AddPendingOp(peerID int, op PendingOperation) {
	mutex.Lock()
	defer mutex.Unlock()
	pendingOps[peerID] = append(pendingOps[peerID], op)
}

// GetAndClearPendingOps obtiene y elimina las operaciones pendientes de un nodo
func GetAndClearPendingOps(peerID int) []PendingOperation {
	mutex.Lock()
	defer mutex.Unlock()
	ops := pendingOps[peerID]
	delete(pendingOps, peerID)
	return ops
}

// PeekPendingOps devuelve las operaciones pendientes sin eliminarlas
func PeekPendingOps(peerID int) []PendingOperation {
	mutex.Lock()
	defer mutex.Unlock()
	return pendingOps[peerID]
}

// GetAllPendingOps devuelve una copia de todas las operaciones pendientes
func GetAllPendingOps() map[int][]PendingOperation {
	mutex.Lock()
	defer mutex.Unlock()

	copyMap := make(map[int][]PendingOperation)
	for peerID, ops := range pendingOps {
		opsCopy := make([]PendingOperation, len(ops))
		copy(opsCopy, ops)
		copyMap[peerID] = opsCopy
	}
	return copyMap
}

