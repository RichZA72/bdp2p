package state

import "time"

// FileInfo reducido para evitar importar fs (y romper ciclos)
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
