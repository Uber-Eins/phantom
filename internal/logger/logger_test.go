package logger

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/op/go-logging"
)

// TestGetLogs_ReturnsAtMostC guards the documented "up to c entries" contract.
// The loop condition must cap output at c (ERROR entries are queried at "debug"
// level so the level filter passes all of them, isolating the count).
func TestGetLogs_ReturnsAtMostC(t *testing.T) {
	logBufferMu.Lock()
	logBuffers[SourceXUI] = nil
	logBufferMu.Unlock()
	for i := range 5 {
		addToBuffer("ERROR", fmt.Sprintf("m%d", i))
	}

	cases := []struct{ c, want int }{
		{0, 0},
		{2, 2},
		{5, 5},
		{10, 5}, // capped at what's available
	}
	for _, tc := range cases {
		if got := GetLogs(tc.c, "debug"); len(got) != tc.want {
			t.Errorf("GetLogs(%d) returned %d entries, want %d", tc.c, len(got), tc.want)
		}
	}
}

func TestGetSourceLogsSeparatesServices(t *testing.T) {
	logBufferMu.Lock()
	logBuffers[SourceXUI] = nil
	logBuffers[SourceXray] = nil
	logBuffers[SourceNginx] = nil
	logBufferMu.Unlock()

	addSourceToBuffer(SourceXUI, logging.INFO, "panel")
	addSourceToBuffer(SourceXray, logging.INFO, "core")
	addSourceToBuffer(SourceNginx, logging.INFO, "front")

	xrayLogs := GetSourceLogs(10, "debug", string(SourceXray))
	if len(xrayLogs) != 1 || xrayLogs[0] == "" {
		t.Fatalf("unexpected Xray logs: %#v", xrayLogs)
	}
	if got := GetLogs(10, "debug"); len(got) != 1 {
		t.Fatalf("panel log count = %d, want 1", len(got))
	}
}

func TestSlogHandlerUsesPanelSource(t *testing.T) {
	logBufferMu.Lock()
	logBuffers[SourceXUI] = nil
	logBufferMu.Unlock()

	slog.New(newSlogHandler()).Error(
		"[sessions] ERROR!",
		"err",
		errors.New("invalid cookie"),
	)

	logs := GetLogs(1, "debug")
	if len(logs) != 1 ||
		!strings.Contains(logs[0], "ERROR [x-ui] [sessions] ERROR! err=invalid cookie") {
		t.Fatalf("unexpected slog bridge output: %#v", logs)
	}
}
