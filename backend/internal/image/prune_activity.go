// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original per-image prune progress for issue #661.

package image

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

// pruneActivityService is the part of the activity service a prune activity writes through.
type pruneActivityService interface {
	activitylib.Service
	UpdateActivity(ctx context.Context, activityID string, req activity.UpdateActivityRequest) (*activitytypes.Activity, error)
}

// pruneActivity mirrors one host prune as an image_prune activity with bulk counts. It starts only
// once the guard holds the image lease and has selected images, so a host runs at most one at a time.
type pruneActivity struct {
	service       pruneActivityService
	environmentID string
	user          *usertypes.Actor
	id            string
	work          context.Context
	planned       int
	report        imagetypes.PruneReport
}

func (p *pruneActivity) Begin(ctx context.Context, planned int) context.Context {
	p.planned = planned
	id, work := activitylib.StartHandlerActivity(ctx, p.service, p.environmentID, activitytypes.TypeImagePrune, "image", "", "", p.user,
		"Removing images", "Image cleanup started", database.JSON{"bulk": p.bulk(imagetypes.PruneReport{})}, false)
	p.id, p.work = id, work
	return work
}

func (p *pruneActivity) Image(ctx context.Context, report *imagetypes.PruneReport) {
	p.report = *report
	if p.id == "" {
		return
	}
	done := len(report.ImagesDeleted) + len(report.Skipped) + len(report.Failed)
	progress := done * 100 / max(p.planned, 1)
	// A failed progress write is logged only; it never repeats or stops a removal.
	if _, err := p.service.UpdateActivity(context.WithoutCancel(ctx), p.id, activity.UpdateActivityRequest{Progress: &progress, Metadata: database.JSON{"bulk": p.bulk(*report)}}); err != nil {
		slog.WarnContext(ctx, "failed to record image prune progress", "activityId", p.id, "error", err)
	}
}

func (p *pruneActivity) bulk(report imagetypes.PruneReport) activitytypes.BulkProgress {
	removed, skipped, failed := len(report.ImagesDeleted), len(report.Skipped), len(report.Failed)
	return activitytypes.BulkProgress{Unit: "target", Planned: p.planned, PlannedKnown: true, Completed: removed + skipped + failed, Succeeded: removed, Failed: failed, Skipped: skipped}
}

// run prunes with this activity as the guard observer and settles it on every exit, panics included.
func (p *pruneActivity) run(ctx context.Context, prune func(context.Context) (*imagetypes.PruneReport, error)) (report *imagetypes.PruneReport, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			p.complete(ctx, fmt.Errorf("image cleanup stopped unexpectedly"))
			panic(recovered)
		}
	}()
	report, err = prune(guard.WithPruneObserver(ctx, p))
	p.complete(ctx, err)
	return report, err
}

// complete settles the activity: an error or cancellation keeps the counts so far; any failed image
// fails the run; otherwise it succeeded, also when every image was skipped.
func (p *pruneActivity) complete(ctx context.Context, err error) {
	if p.id == "" {
		return
	}
	if err == nil && len(p.report.Failed) > 0 {
		err = fmt.Errorf("%d of %d images could not be removed", len(p.report.Failed), p.planned)
	}
	// The work context carries an activity cancel request; the request context a client disconnect.
	if p.work.Err() != nil {
		ctx = p.work
	}
	activitylib.CompleteHandlerActivity(ctx, p.service, p.id, fmt.Sprintf("%d images removed", len(p.report.ImagesDeleted)), err)
}
