package runtime

import (
	"sync"
)

// Manager exposes the one local runtime used by the single-machine panel.
type Manager struct {
	local Runtime

	mu            sync.RWMutex
	localOverride Runtime
}

func NewManager(localDeps LocalDeps) *Manager {
	return &Manager{local: NewLocal(localDeps)}
}

// SetLocalRuntimeOverride is a test seam for local runtime operations.
func (m *Manager) SetLocalRuntimeOverride(rt Runtime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.localOverride = rt
}

func (m *Manager) Runtime() Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.localOverride != nil {
		return m.localOverride
	}
	return m.local
}

var (
	managerMu sync.RWMutex
	manager   *Manager
)

func SetManager(value *Manager) {
	managerMu.Lock()
	defer managerMu.Unlock()
	manager = value
}

func GetManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return manager
}
