// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: In-process slot provenance preserves cleanup when ownership reads fail.

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
