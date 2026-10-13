// Source: getarcaneapp/arcane/cli/internal/runstate/state.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package runstate

import (
	"sync"
	"time"
)

type State struct {
	EnvOverride    string
	OutputMode     string
	AssumeYes      bool
	NoColor        bool
	RequestTimeout time.Duration
}

var (
	mu    sync.RWMutex
	state State
)

func Set(next State) {
	mu.Lock()
	defer mu.Unlock()
	state = next
}

func Get() State {
	mu.RLock()
	defer mu.RUnlock()
	return state
}
