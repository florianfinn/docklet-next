// Source: docklet-next/issues/108
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity list source-completeness contract and regression evidence.

package activity

import (
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
)

// ListActivitiesOutput adds source completeness metadata only to activity lists.
type ListActivitiesOutput struct {
	HostReason string `header:"X-Docklet-Host-Reason" required:"false" enum:"agent.unavailable" doc:"Present only on HTTP 200 when remote transport is unavailable and the page contains only retained hub summaries (possibly empty). Any marked page makes the whole host read incomplete; retain previous agent observations until a complete successful host read. Omitted on complete local or remote reads and on errors. This is not an observation timestamp or execution outcome."`
	Body       base.Paginated[activity.Activity]
}
