// Source: getarcaneapp/kit/updater/memory_store.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package updater

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"
)

type memoryPendingStore struct {
	mu      sync.Mutex
	records map[string]ImageUpdateRecord
}

// NewMemoryPendingStore returns an in-memory pending update store.
func NewMemoryPendingStore(records ...ImageUpdateRecord) PendingStore {
	store := &memoryPendingStore{records: make(map[string]ImageUpdateRecord, len(records))}
	for _, record := range records {
		store.records[memoryPendingStoreKey(record)] = record
	}
	return store
}

func (s *memoryPendingStore) PendingImageUpdates(ctx context.Context) ([]ImageUpdateRecord, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]ImageUpdateRecord, 0, len(s.records))
	for _, key := range slices.Sorted(maps.Keys(s.records)) {
		out = append(out, s.records[key])
	}
	return out, nil
}

func (s *memoryPendingStore) ClearImageUpdateRecord(ctx context.Context, record ImageUpdateRecord) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, memoryPendingStoreKey(record))
	return nil
}

func memoryPendingStoreKey(record ImageUpdateRecord) string {
	key := record.ID
	if key == "" {
		key = record.ImageRef()
	}
	return strconv.Quote(record.ContainerID) + ":" + strconv.Quote(key)
}
