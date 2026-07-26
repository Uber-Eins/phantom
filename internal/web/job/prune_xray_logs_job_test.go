package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeLogConfig(t *testing.T, accessPath string, errorPath string) {
	t.Helper()
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	configData, err := json.Marshal(map[string]any{
		"log": map[string]any{"access": accessPath, "error": errorPath},
	})
	if err != nil {
		t.Fatalf("marshal xray config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), configData, 0o644); err != nil {
		t.Fatalf("write xray config: %v", err)
	}
}

func TestPruneXrayLogsTruncatesOnlyOversizedLogs(t *testing.T) {
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	defer func() { maxXrayLogBytes = oldMax }()

	dir := t.TempDir()
	accessLog := filepath.Join(dir, "access.log")
	errorLog := filepath.Join(dir, "error.log")
	if err := os.WriteFile(accessLog, []byte("small"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("large log line"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	NewPruneXrayLogsJob().Run()

	accessInfo, err := os.Stat(accessLog)
	if err != nil {
		t.Fatalf("access log should still exist: %v", err)
	}
	if accessInfo.Size() != 5 {
		t.Fatalf("small access log should be left alone, got %d bytes", accessInfo.Size())
	}
	errorInfo, err := os.Stat(errorLog)
	if err != nil {
		t.Fatalf("error log should still exist: %v", err)
	}
	if errorInfo.Size() != 0 {
		t.Fatalf("oversized error log should be truncated, got %d bytes", errorInfo.Size())
	}
}
