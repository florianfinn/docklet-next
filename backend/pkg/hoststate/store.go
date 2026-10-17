// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package hoststate

import (
	"sync"
	"time"
)

type Key struct {
	HostID string
	List   string
}
type Snapshot struct {
	Data       []byte
	ObservedAt time.Time
}

// Store retains successful reads for one hub process. A new store has no stale lists.
type Store struct {
	mu     sync.RWMutex
	values map[Key]Snapshot
}

func (s *Store) Put(key Key, data []byte) Snapshot {
	value := Snapshot{Data: append([]byte(nil), data...), ObservedAt: time.Now().UTC()}
	s.mu.Lock()
	if s.values == nil {
		s.values = make(map[Key]Snapshot)
	}
	s.values[key] = value
	s.mu.Unlock()
	return clone(value)
}
func (s *Store) Get(key Key) (Snapshot, bool) {
	s.mu.RLock()
	value, ok := s.values[key]
	s.mu.RUnlock()
	return clone(value), ok
}
func (s *Store) Forget(hostID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.values {
		if key.HostID == hostID {
			delete(s.values, key)
		}
	}
}
func clone(value Snapshot) Snapshot { value.Data = append([]byte(nil), value.Data...); return value }
