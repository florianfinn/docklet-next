// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package updater

import (
	"context"
	"strconv"
)

func (s *UpdaterService) minimumAgeDays(ctx context.Context) int {
	n, e := strconv.Atoi(s.deps.Settings.GetStringSetting(ctx, "updateMinimumAgeDays", "0"))
	if e != nil || n < 0 || n > 90 {
		return 90
	}
	return n
}
