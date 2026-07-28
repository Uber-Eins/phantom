// Package logger provides logging functionality for the 3x-ui panel with
// dual-backend logging (console/syslog and file) and buffered log storage for web UI.
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/op/go-logging"

	"github.com/Uber-Eins/phantom/v3/internal/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxLogBufferSize = 10240                 // Maximum log entries kept in memory
	logFileName      = "3xui.log"            // Log file name
	timeFormat       = "2006/01/02 15:04:05" // Log timestamp format

	// On-disk rotation limits — single file capped, old segments pruned automatically.
	maxLogFileMB    = 10 // rotate active log when larger than this
	maxLogBackups   = 5  // rotated files retained (beyond current segment)
	maxLogAgeDays   = 7  // remove rotated backups older than this (0 disables time-based pruning)
	compressRotated = true
)

// Source identifies the process that emitted a log entry.
type Source string

const (
	SourceXUI   Source = "x-ui"
	SourceXray  Source = "xray-core"
	SourceNginx Source = "nginx"
)

var (
	// Initialized to a usable default so logging never nil-derefs before InitLogger
	// runs — the "migrate" and "setting" CLI subcommands log without calling it.
	logger        = logging.MustGetLogger(string(SourceXUI))
	sourceLoggers = map[Source]*logging.Logger{
		SourceXUI:   logger,
		SourceXray:  logging.MustGetLogger(string(SourceXray)),
		SourceNginx: logging.MustGetLogger(string(SourceNginx)),
	}
	fileRotate *lumberjack.Logger // nil when file backend disabled
)

// InitLogger initializes dual logging backends: console/syslog and file.
// Console logging uses the specified level, file logging always uses DEBUG level.
func InitLogger(level logging.Level) {
	backends := make([]logging.Backend, 0, 2)

	// Console/syslog backend with configurable level
	consoleBackend, consoleErr := initDefaultBackend()
	leveledBackend := logging.AddModuleLevel(consoleBackend)
	for source := range sourceLoggers {
		leveledBackend.SetLevel(level, string(source))
	}
	backends = append(backends, leveledBackend)

	// File backend with DEBUG level for comprehensive logging
	fileBackend, fileErr := initFileBackend()
	if fileBackend != nil {
		leveledBackend := logging.AddModuleLevel(fileBackend)
		for source := range sourceLoggers {
			leveledBackend.SetLevel(logging.DEBUG, string(source))
		}
		backends = append(backends, leveledBackend)
	}

	multiBackend := logging.MultiLogger(backends...)
	for _, sourceLogger := range sourceLoggers {
		sourceLogger.SetBackend(multiBackend)
	}
	logger = sourceLoggers[SourceXUI]
	slog.SetDefault(slog.New(newSlogHandler()))

	if consoleErr != nil {
		Warning("syslog backend disabled:", consoleErr)
	}
	if fileErr != nil {
		Warning("file log backend disabled:", fileErr)
	}
}

// initDefaultBackend creates the console/syslog logging backend.
// Windows: Uses stderr directly (no syslog support)
// Unix-like: Attempts syslog, falls back to stderr
func initDefaultBackend() (logging.Backend, error) {
	var backend logging.Backend
	includeTime := false
	var backendErr error

	if runtime.GOOS == "windows" {
		// Windows: Use stderr directly (no syslog support)
		backend = logging.NewLogBackend(os.Stderr, "", 0)
		includeTime = true
	} else {
		// Unix-like: Try syslog, fallback to stderr
		if syslogBackend, err := logging.NewSyslogBackend(""); err != nil {
			backendErr = err
			backend = logging.NewLogBackend(os.Stderr, "", 0)
			includeTime = true
		} else {
			backend = syslogBackend
		}
	}

	return logging.NewBackendFormatter(backend, newFormatter(includeTime)), backendErr
}

// initFileBackend creates the file logging backend with size/age‑bounded rotation
// so log volume cannot grow without limit on disk.
func initFileBackend() (logging.Backend, error) {
	logDir := config.GetLogFolder()
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("create log folder %s: %w", logDir, err)
	}

	logPath := filepath.Join(logDir, logFileName)
	fileRotate = &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxLogFileMB,
		MaxBackups: maxLogBackups,
		MaxAge:     maxLogAgeDays,
		LocalTime:  true,
		Compress:   compressRotated,
	}

	backend := logging.NewLogBackend(fileRotate, "", 0)
	return logging.NewBackendFormatter(backend, newFormatter(true)), nil
}

// newFormatter creates a log formatter with optional timestamp.
func newFormatter(withTime bool) logging.Formatter {
	format := `%{level} [%{module}] %{message}`
	if withTime {
		format = `%{time:` + timeFormat + `} %{level} [%{module}] %{message}`
	}
	return logging.MustStringFormatter(format)
}

// CloseLogger closes the rotating log writer and cleans up resources.
// Should be called during application shutdown.
func CloseLogger() {
	if fileRotate != nil {
		_ = fileRotate.Close()
		fileRotate = nil
	}
}

// Debug logs a debug message and adds it to the log buffer.
func Debug(args ...any) {
	write(SourceXUI, logging.DEBUG, formatArgs(args...))
}

// Debugf logs a formatted debug message and adds it to the log buffer.
func Debugf(format string, args ...any) {
	write(SourceXUI, logging.DEBUG, fmt.Sprintf(format, args...))
}

// Info logs an info message and adds it to the log buffer.
func Info(args ...any) {
	write(SourceXUI, logging.INFO, formatArgs(args...))
}

// Infof logs a formatted info message and adds it to the log buffer.
func Infof(format string, args ...any) {
	write(SourceXUI, logging.INFO, fmt.Sprintf(format, args...))
}

// Notice logs a notice message and adds it to the log buffer.
func Notice(args ...any) {
	write(SourceXUI, logging.NOTICE, formatArgs(args...))
}

// Noticef logs a formatted notice message and adds it to the log buffer.
func Noticef(format string, args ...any) {
	write(SourceXUI, logging.NOTICE, fmt.Sprintf(format, args...))
}

// Warning logs a warning message and adds it to the log buffer.
func Warning(args ...any) {
	write(SourceXUI, logging.WARNING, formatArgs(args...))
}

// Warningf logs a formatted warning message and adds it to the log buffer.
func Warningf(format string, args ...any) {
	write(SourceXUI, logging.WARNING, fmt.Sprintf(format, args...))
}

// Error logs an error message and adds it to the log buffer.
func Error(args ...any) {
	write(SourceXUI, logging.ERROR, formatArgs(args...))
}

// Errorf logs a formatted error message and adds it to the log buffer.
func Errorf(format string, args ...any) {
	write(SourceXUI, logging.ERROR, fmt.Sprintf(format, args...))
}

func SourceDebug(source Source, args ...any) {
	write(source, logging.DEBUG, formatArgs(args...))
}

func SourceInfo(source Source, args ...any) {
	write(source, logging.INFO, formatArgs(args...))
}

func SourceNotice(source Source, args ...any) {
	write(source, logging.NOTICE, formatArgs(args...))
}

func SourceWarning(source Source, args ...any) {
	write(source, logging.WARNING, formatArgs(args...))
}

func SourceError(source Source, args ...any) {
	write(source, logging.ERROR, formatArgs(args...))
}

func formatArgs(args ...any) string {
	return strings.TrimSuffix(fmt.Sprintln(args...), "\n")
}

func write(source Source, level logging.Level, message string) {
	target, ok := sourceLoggers[source]
	if !ok {
		source = SourceXUI
		target = logger
	}
	switch level {
	case logging.DEBUG:
		target.Debug(message)
	case logging.INFO:
		target.Info(message)
	case logging.NOTICE:
		target.Notice(message)
	case logging.WARNING:
		target.Warning(message)
	default:
		target.Error(message)
	}
	addSourceToBuffer(source, level, message)
}
