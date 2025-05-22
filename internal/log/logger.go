package log

import (
    "encoding/json"
    "os"
    "time"
)

type LogEntry struct {
    Time     time.Time `json:"time"`
    Action   string    `json:"action"`   // "CREATE", "DELETE", "TRANSFER"
    FileName string    `json:"fileName"`
    OriginID int       `json:"originID"`
    TargetID int       `json:"targetID"`
}

var logFile = "shared/log.json"

func AppendLog(entry LogEntry) {
    var logs []LogEntry
    file, _ := os.ReadFile(logFile)
    _ = json.Unmarshal(file, &logs)
    logs = append(logs, entry)
    data, _ := json.MarshalIndent(logs, "", "  ")
    _ = os.WriteFile(logFile, data, 0644)
}

func GetLogs() []LogEntry {
    var logs []LogEntry
    file, err := os.ReadFile(logFile)
    if err == nil {
        _ = json.Unmarshal(file, &logs)
    }
    return logs
}
