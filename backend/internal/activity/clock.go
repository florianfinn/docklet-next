// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 660e9e31bfeb0c724d61ee5d43aec3288c84289d
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Injectable process clock for gate skew regression tests.

package activity

import "time"

func (s *ActivityService) nowInternal() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}
