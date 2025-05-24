package log

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"p2pfs/internal/peer"
)

type LogEntry struct {
	Time     time.Time `json:"time"`
	Action   string    `json:"action"`   // "CREATE", "DELETE", "TRANSFER"
	FileName string    `json:"fileName"`
	OriginID int       `json:"originID"`
	TargetID int       `json:"targetID"`
}

var logFile = "shared/log.json"

// AppendLog agrega una entrada al archivo de logs
func AppendLog(entry LogEntry) error {
	var logs []LogEntry
	file, err := os.ReadFile(logFile)
	if err == nil {
		_ = json.Unmarshal(file, &logs)
	}
	logs = append(logs, entry)
	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(logFile, data, 0644)
}

// GetLogs devuelve todas las entradas de log
func GetLogs() []LogEntry {
	var logs []LogEntry
	file, err := os.ReadFile(logFile)
	if err == nil {
		_ = json.Unmarshal(file, &logs)
	}
	return logs
}

// SendLogsToPeer envía los logs locales al peer destino
func SendLogsToPeer(pinfo peer.PeerInfo) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", pinfo.IP, pinfo.Port))
	if err != nil {
		fmt.Println("❌ No se pudo conectar a", pinfo.IP, "para enviar logs.")
		return
	}
	defer conn.Close()

	logs := GetLogs()
	msg := map[string]interface{}{
		"type": "SYNC_LOGS",
		"logs": logs,
	}

	err = json.NewEncoder(conn).Encode(msg)
	if err != nil {
		fmt.Println("❌ Error al enviar logs:", err)
	} else {
		fmt.Println("📤 Logs enviados a Maq", pinfo.ID)
	}
}
