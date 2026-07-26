package panel

import (
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
)

// PanelService provides process-level panel controls.
type PanelService struct{}

func (s *PanelService) RestartPanel(delay time.Duration) error {
	go func() {
		time.Sleep(delay)
		if global.TriggerRestart() {
			return
		}
		if runtime.GOOS == "windows" {
			logger.Error("panel restart: no restart hook registered")
			return
		}
		process, err := os.FindProcess(syscall.Getpid())
		if err != nil {
			logger.Error("panel restart: find process failed:", err)
			return
		}
		if err := process.Signal(syscall.SIGHUP); err != nil {
			logger.Error("panel restart: signal failed:", err)
		}
	}()
	return nil
}
