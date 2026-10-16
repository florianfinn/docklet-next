// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package apikey

import "time"

// InvalidateAfterRestore discards records from the replaced database and blocks
// stale validations from repopulating the cache after the maintenance barrier.
func (s *ApiKeyService) InvalidateAfterRestore() {
	if s == nil || s.validatedKeyCache == nil {
		return
	}
	s.cacheFillMu.Lock()
	s.cacheGen.Add(1)
	var keys []string
	s.validatedKeyCache.Range(func(key string, _ ApiKey) bool { keys = append(keys, key); return true })
	s.validatedKeyCache.DeleteMany(keys)
	s.cacheFillMu.Unlock()
	s.lastUsedMu.Lock()
	s.lastUsedWrites = make(map[string]time.Time)
	s.lastUsedMu.Unlock()
}
