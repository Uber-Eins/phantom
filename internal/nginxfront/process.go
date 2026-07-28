package nginxfront

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Uber-Eins/phantom/v3/internal/database"
	"github.com/Uber-Eins/phantom/v3/internal/logger"
)

const stopTimeout = 5 * time.Second

var nginxNativeLog = regexp.MustCompile(
	`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \[([a-z]+)\] \d+#\d+: (.*)$`,
)

type processManager struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
	exit chan error
}

var manager processManager

func nginxBinary() string {
	if value := os.Getenv("XUI_NGINX_BINARY"); value != "" {
		return value
	}
	return "/usr/sbin/nginx"
}

func PrepareRuntimeDir() error {
	if err := os.MkdirAll(RunDir(), 0o700); err != nil {
		return err
	}
	routes, err := loadRoutes(database.GetDB(), true)
	if err != nil {
		return err
	}
	for _, item := range routes {
		if err := os.MkdirAll(filepath.Dir(item.Socket), 0o750); err != nil {
			return err
		}
	}
	return nil
}

// Reconcile validates and atomically applies the Nginx configuration derived
// from enabled guided inbounds. With no enabled routes, it stops Nginx.
func Reconcile() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	routes, err := loadRoutes(database.GetDB(), true)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		return manager.stopLocked()
	}
	if err := PrepareRuntimeDir(); err != nil {
		return err
	}
	if err := writeInlineCertificates(routes); err != nil {
		return err
	}
	configText, err := renderConfig(routes)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(ConfigDir(), "nginx-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, configText); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := testConfig(tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, ConfigPath()); err != nil {
		return err
	}

	if manager.runningLocked() {
		if err := manager.cmd.Process.Signal(syscall.SIGHUP); err != nil {
			return fmt.Errorf("reload nginx: %w", err)
		}
		logger.Info("nginx fronting configuration reloaded")
		return nil
	}
	for _, path := range []string{httpsSocket(), H1FallbackSocket(), H2FallbackSocket()} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return manager.startLocked()
}

func testConfig(path string) error {
	var output bytes.Buffer
	cmd := exec.Command(nginxBinary(), "-t", "-q", "-c", path)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nginx config test failed: %w: %s", err, output.String())
	}
	return nil
}

func (m *processManager) startLocked() error {
	cmd := exec.Command(
		nginxBinary(),
		"-c", ConfigPath(),
	)
	cmd.Stdout = &nginxLogWriter{}
	cmd.Stderr = &nginxLogWriter{}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start nginx: %w", err)
	}
	m.cmd = cmd
	m.done = make(chan struct{})
	m.exit = make(chan error, 1)
	go m.wait(cmd, m.done, m.exit)
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-m.exit:
		m.cmd = nil
		m.done = nil
		m.exit = nil
		if err == nil {
			return errors.New("nginx exited during startup")
		}
		return fmt.Errorf("nginx exited during startup: %w", err)
	case <-timer.C:
	}
	logger.Info("nginx fronting started on TCP/443")
	return nil
}

func (m *processManager) wait(cmd *exec.Cmd, done chan struct{}, result chan<- error) {
	err := cmd.Wait()
	result <- err
	close(done)
	if err != nil {
		logger.Warning("nginx fronting exited:", err)
	}
}

func (m *processManager) runningLocked() bool {
	if m.cmd == nil || m.cmd.Process == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

func (m *processManager) stopLocked() error {
	if !m.runningLocked() {
		m.cmd = nil
		m.done = nil
		m.exit = nil
		return nil
	}
	if err := m.cmd.Process.Signal(syscall.SIGQUIT); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	timer := time.NewTimer(stopTimeout)
	defer timer.Stop()
	select {
	case <-m.done:
	case <-timer.C:
		if err := m.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		<-m.done
	}
	m.cmd = nil
	m.done = nil
	m.exit = nil
	logger.Info("nginx fronting stopped")
	return nil
}

// Stop terminates the managed Nginx process.
func Stop() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.stopLocked()
}

type nginxLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *nginxLogWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	count := len(data)
	w.buf.Write(data)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			w.buf.WriteString(line)
			break
		}
		if line != "" {
			writeNginxLog(strings.TrimSpace(line))
		}
	}
	return count, nil
}

func writeNginxLog(line string) {
	if line == "" {
		return
	}
	level := "info"
	body := line
	if match := nginxNativeLog.FindStringSubmatch(line); len(match) == 3 {
		level = match[1]
		body = match[2]
	}

	switch level {
	case "debug":
		logger.SourceDebug(logger.SourceNginx, body)
	case "notice":
		logger.SourceNotice(logger.SourceNginx, body)
	case "warn":
		logger.SourceWarning(logger.SourceNginx, body)
	case "error", "crit", "alert", "emerg":
		logger.SourceError(logger.SourceNginx, body)
	default:
		logger.SourceInfo(logger.SourceNginx, body)
	}
}
