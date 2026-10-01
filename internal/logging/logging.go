// Package logging provides file-based structured logging for avredit.
//
// Design:
//   - Uses log/slog (stdlib) for structured, leveled logging
//   - Uses lumberjack for automatic log rotation
//   - Log level configurable via config.json ("log_level") and CLI flag (--log-level)
//   - Runtime level change via :log-level command or SetLevel()
//   - Default level: ERROR (minimal noise for end users)
//   - Log location: ~/.config/avredit/log/avredit.log
//
// Rotation policy:
//   - Max file size: 10 MB (rotates when exceeded)
//
// Integration:
//   - Call Init() early in main.go after config load
//   - Use slog.Debug/Info/Warn/Error throughout the codebase
//   - Call SetLevel() from TUI command to change at runtime
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// MaxSizeMB is the maximum log file size before rotation.
	MaxSizeMB = 10
)

// levelVar holds the current log level, changeable at runtime.
var levelVar slog.LevelVar

// ParseLevel converts a string level name to slog.Level.
// Accepted values: "debug", "info", "warn", "error" (case-insensitive).
// Returns slog.LevelError for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "":
		return slog.LevelError
	default:
		return slog.LevelError
	}
}

// LevelName returns the canonical string name for a log level.
func LevelName(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "debug"
	case l <= slog.LevelInfo:
		return "info"
	case l <= slog.LevelWarn:
		return "warn"
	default:
		return "error"
	}
}

// SetLevel changes the log level at runtime without reinitializing.
func SetLevel(level slog.Level) {
	levelVar.Set(level)
	slog.Info("log level changed to " + LevelName(level))
}

// Level returns the current active log level.
func Level() slog.Level {
	return levelVar.Level()
}

// Init configures the global slog logger with file output and rotation.
// logDir is the directory for log files (created if needed).
// level controls the initial minimum log level.
// Returns a cleanup function to close the log writer.
func Init(logDir string, level slog.Level) (cleanup func(), err error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	logFile := filepath.Join(logDir, "avredit.log")

	writer := &lumberjack.Logger{
		Filename:  logFile,
		MaxSize:   MaxSizeMB,
		LocalTime: true,
	}

	levelVar.Set(level)

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: &levelVar,
	})

	slog.SetDefault(slog.New(handler))
	slog.Debug("logging initialized")

	return func() { writer.Close() }, nil
}

// LogDir returns the default log directory (~/.config/avredit/log/).
func LogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "log")
	}
	return filepath.Join(home, ".config", "avredit", "log")
}
