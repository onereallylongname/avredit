package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelError},
		{"unknown", slog.LevelError},
		{"  debug  ", slog.LevelDebug},
	}

	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestLevelName(t *testing.T) {
	tests := []struct {
		input    slog.Level
		expected string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
	}

	for _, tt := range tests {
		got := LevelName(tt.input)
		if got != tt.expected {
			t.Errorf("LevelName(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "log")

	cleanup, err := Init(logDir, slog.LevelDebug)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer cleanup()

	// Verify log directory was created
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Fatal("log directory was not created")
	}

	// Verify log file was created after writing
	slog.Info("test message", "key", "value")
	logFile := filepath.Join(logDir, "avredit.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatal("log file was not created")
	}

	// Verify content was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty")
	}
}

func TestLogDir(t *testing.T) {
	dir := LogDir()
	if dir == "" {
		t.Fatal("LogDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		// On systems with HOME set, should be absolute
		home, _ := os.UserHomeDir()
		if home != "" {
			t.Errorf("LogDir() = %q, expected absolute path", dir)
		}
	}
}
