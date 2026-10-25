// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Neutral image audit and canonical-ID cleanup for issue #229.

package image

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/user"
	"regexp"
)

// Audit shares the removal hook across interactive and scheduled callers. Only
// validated references and typed outcomes enter event history; never daemon text.
func (s *ImageService) auditImageMutation(ctx context.Context, action, id, ref string, operationErr error) {
	if s.eventService == nil {
		return
	}
	actor := &user.SystemUser
	if current, ok := userctx.CurrentUserFromContext(ctx); ok && current != nil {
		actor = current
	}
	if guard.ValidateReference(ref) != nil {
		ref = ""
	}
	if !auditImageID.MatchString(id) {
		id = ""
	}
	kind := map[string]event.EventType{"pull": event.EventTypeImagePull, "delete": event.EventTypeImageDelete, "tag": event.EventTypeImageTag, "load": event.EventTypeImageLoad, "bulk_prune": event.EventTypeImagePrune}[action]
	metadata := database.JSON{"action": action, "id": id, "reference": ref, "code": "image." + action + "-succeeded"}
	ctx = context.WithoutCancel(ctx)
	if operationErr != nil {
		fallback := "image." + action + "-failed"
		if action == "delete" {
			fallback = "image.remove-failed"
		}
		if action == "bulk_prune" {
			fallback = "image.prune-failed"
		}
		problem := imageDTO.Neutral(operationErr, fallback, 502)
		metadata["code"] = problem.Problem.Code
		s.eventService.LogErrorEvent(ctx, kind, "image", id, ref, actor.ID, actor.Username, "0", problem, metadata)
		return
	}
	_ = s.eventService.LogImageEvent(ctx, kind, id, ref, actor.ID, actor.Username, "0", metadata)
}

func (s *ImageService) removedImage(ctx context.Context, id string) {
	s.cleanupImageUpdateRecordsInternal(ctx, []string{id})
	s.cleanupVulnerabilityRecordsAfterPruneInternal(ctx, []string{id})
}

var auditImageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
