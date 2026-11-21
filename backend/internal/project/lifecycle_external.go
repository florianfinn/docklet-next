// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Immutable lifecycle external dependency barriers; M34 #548.

package project

import (
	"context"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"slices"
)

func lifecycleSavedRevision(snapshot *life.Snapshot) string {
	if len(snapshot.External) > 0 {
		return life.Hash([]any{snapshot.Files, snapshot.Model, snapshot.External})
	}
	return life.Hash([]any{snapshot.Files, snapshot.Model})
}
func lifecycleExternalEqual(a, b *life.Snapshot) bool { return slices.Equal(a.External, b.External) }

func (s *ProjectService) checkLifecycleExternal(ctx context.Context, snapshots ...*life.Snapshot) error {
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.Model == nil {
			return lifecycleFailure("update.target-changed")
		}
		root, err := s.GetProjectsDirectory(ctx)
		if err != nil {
			return lifecycleFailure("update.target-changed")
		}
		input, err := projects.FreshComposeInputs(s.composeInputContext(ctx), root, snapshot.Model.WorkingDir, false)
		if err != nil {
			if len(snapshot.External) == 0 && (input == nil || input.State == "none") {
				continue
			}
			return lifecycleFailure("update.target-changed")
		}
		current := make([]life.ExternalInput, 0, len(input.External))
		for _, entry := range input.External {
			current = append(current, life.ExternalInput{Path: entry.Path, SHA256: entry.SHA256})
		}
		if !slices.Equal(current, snapshot.External) {
			return lifecycleFailure("update.target-changed")
		}
	}
	return nil
}

func (s *ProjectService) checkIntentExternal(ctx context.Context, id, registration string, intent *dto.LifecycleIntent) error {
	if intent == nil {
		return nil
	}
	if intent.Request.PointID != "" {
		_, saved, err := s.lifecycleStore().Point(ctx, intent.Request.PointID)
		if err == nil {
			return s.checkLifecycleExternal(ctx, saved)
		}
	} else {
		saved, _, err := s.lifecycleStore().Baseline(ctx, &life.Snapshot{TargetID: id, TargetIdentity: intent.TargetIdentity, RegistrationID: registration})
		if err == nil {
			return s.checkLifecycleExternal(ctx, saved)
		}
	}
	return nil
}
