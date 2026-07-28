package nginxfront

import (
	"strings"
	"testing"

	"github.com/Uber-Eins/phantom/v3/internal/logger"
)

func TestWriteNginxLogNormalizesNativeLine(t *testing.T) {
	writeNginxLog(
		"2026/07/27 15:13:42 [notice] 68#68: using the \"epoll\" event method",
	)

	logs := logger.GetSourceLogs(10, "debug", string(logger.SourceNginx))
	if len(logs) == 0 ||
		!strings.Contains(logs[0], "NOTICE [nginx] using the \"epoll\" event method") {
		t.Fatalf("unexpected Nginx logs: %#v", logs)
	}
}
