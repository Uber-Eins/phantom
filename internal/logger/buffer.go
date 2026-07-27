package logger

import (
	"fmt"
	"sync"
	"time"

	"github.com/op/go-logging"
)

type logEntry struct {
	time   string
	level  logging.Level
	source Source
	log    string
}

var (
	// Each service has its own bounded in-memory log so a noisy process cannot
	// evict another service's entries from the web UI.
	logBufferMu sync.Mutex
	logBuffers  = map[Source][]logEntry{
		SourceXUI:   {},
		SourceXray:  {},
		SourceNginx: {},
	}
)

// addToBuffer is retained for package tests and panel-only callers.
func addToBuffer(level string, newLog string) {
	logLevel, _ := logging.LogLevel(level)
	addSourceToBuffer(SourceXUI, logLevel, newLog)
}

func addSourceToBuffer(source Source, level logging.Level, newLog string) {
	logBufferMu.Lock()
	defer logBufferMu.Unlock()
	buffer := logBuffers[source]
	if len(buffer) >= maxLogBufferSize {
		buffer = buffer[1:]
	}
	logBuffers[source] = append(buffer, logEntry{
		time:   time.Now().Format(timeFormat),
		level:  level,
		source: source,
		log:    newLog,
	})
}

// GetLogs retrieves up to c panel log entries at or below the specified level.
func GetLogs(c int, level string) []string {
	return GetSourceLogs(c, level, string(SourceXUI))
}

// GetSourceLogs retrieves one service's recent logs for the web UI.
func GetSourceLogs(c int, level string, sourceName string) []string {
	var output []string
	logLevel, _ := logging.LogLevel(level)
	source := normalizeSource(sourceName)

	// Snapshot under the lock, then filter and format without blocking writers.
	logBufferMu.Lock()
	snapshot := append([]logEntry(nil), logBuffers[source]...)
	logBufferMu.Unlock()

	for i := len(snapshot) - 1; i >= 0 && len(output) < c; i-- {
		if snapshot[i].level <= logLevel {
			output = append(output, fmt.Sprintf(
				"%s %s [%s] %s",
				snapshot[i].time,
				snapshot[i].level,
				snapshot[i].source,
				snapshot[i].log,
			))
		}
	}
	return output
}

func normalizeSource(source string) Source {
	switch Source(source) {
	case SourceXray:
		return SourceXray
	case SourceNginx:
		return SourceNginx
	default:
		return SourceXUI
	}
}
