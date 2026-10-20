// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted slot registration; in-process runtime ownership preserves legacy cleanup on read faults.

package activity

// Slots share a limiter but have different release authorities. Preserve the
// registering owner's kind independently of later database availability.
type activitySlotReleaseInternal struct {
	release func()
	runtime bool
}

func (s *ActivityService) registerOwnedSlotReleaseInternal(id string, release func(), runtime bool) {
	s.slotMu.Lock()
	defer s.slotMu.Unlock()
	if existing, ok := s.slotReleases[id]; ok {
		existing.release()
	}
	s.slotReleases[id] = activitySlotReleaseInternal{release: release, runtime: runtime}
}

// Caller holds runningMu. Runtime owners never use the legacy running registry;
// their slots remain owned through failed terminal persistence and output join.
func (s *ActivityService) knownLegacyOwnerInternal(id string) bool {
	if s.runtime != nil && s.runtime.owners[id] != nil {
		return false
	}
	s.slotMu.Lock()
	slot, held := s.slotReleases[id]
	s.slotMu.Unlock()
	if held {
		return !slot.runtime
	}
	return s.running[id] != nil
}
