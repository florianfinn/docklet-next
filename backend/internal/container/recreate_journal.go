// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original durable recreate effects and exact-identity startup compensation.

package container

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	dto "github.com/florianfinn/docklet-next/types/container"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

// recreateEffectTimeout bounds the detached effect and compensation sequence.
const recreateEffectTimeout = 5 * time.Minute

type recreateUncertain struct{ cause error }

func (e *recreateUncertain) Error() string          { return "container.recreate-ambiguous" }
func (e *recreateUncertain) RuntimeUncertain() bool { return true }
func (e *recreateUncertain) Unwrap() error          { return e.cause }
func (s *ContainerService) saveRecreate(ctx context.Context, j *RecreateJournal, phase string) error {
	j.Phase, j.UpdatedAt = phase, time.Now().UTC()
	if s.operations == nil || s.operations.db == nil {
		return &recreateUncertain{cause: errors.New("recreate journal unavailable")}
	}
	if err := s.operations.db.WithContext(ctx).Save(j).Error; err != nil {
		return &recreateUncertain{cause: err}
	}
	return nil
}
func (s *ContainerService) recreateDurable(ctx context.Context, c *client.Client, old dockercontainer.InspectResponse, name string, config *dockercontainer.Config, host *dockercontainer.HostConfig, networks *network.NetworkingConfig, apiVersion string) (string, error) {
	run := updaterDTO.ExecutionIdentityFromContext(ctx).RunID
	// Cancellation and the run deadline are honoured only before the first
	// journal write; afterwards the sequence and its compensation run detached.
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recreateEffectTimeout)
	defer cancel()
	j := &RecreateJournal{RunID: run, OldID: old.ID, OldName: strings.TrimPrefix(old.Name, "/"), BackupName: strings.TrimPrefix(old.Name, "/") + "-docklet-recreate-" + run, NewName: name, WasRunning: old.State != nil && old.State.Running, AutoRemove: old.HostConfig != nil && old.HostConfig.AutoRemove}
	if err := s.saveRecreate(ctx, j, "intent-rename"); err != nil {
		return "", err
	}
	fail := func(step string, effectErr error) (string, error) {
		if ctx.Err() != nil {
			return "", &recreateUncertain{cause: effectErr}
		}
		recovery, err := s.recoverRecreate(ctx, c, j)
		if err != nil {
			// A failed recovery read is no outcome: the journal stays unresolved and
			// the run stays held for startup recovery.
			return "", &recreateUncertain{cause: err}
		}
		if recovery.Result != nil {
			dto.RecordRecreateResult(ctx, *recovery.Result)
			return recovery.Result.NewID, nil
		}
		return "", dto.RecreateFailure(recovery.Code, step)
	}
	if _, err := c.ContainerRename(ctx, j.OldID, client.ContainerRenameOptions{NewName: j.BackupName}); err != nil {
		return fail("rename", err)
	}
	if err := s.saveRecreate(ctx, j, "renamed"); err != nil {
		return "", err
	}
	// intent-create covers stopping the original as preparation for create. The
	// observed write after stop preserves the recovery timestamp before creation.
	if j.WasRunning && !j.AutoRemove {
		if err := s.saveRecreate(ctx, j, "intent-create"); err != nil {
			return "", err
		}
		if _, err := c.ContainerStop(ctx, j.OldID, client.ContainerStopOptions{Timeout: new(30)}); err != nil {
			return fail("stop", err)
		}
		if err := s.saveRecreate(ctx, j, "intent-create"); err != nil {
			return "", err
		}
	}
	if err := s.saveRecreate(ctx, j, "intent-create"); err != nil {
		return "", err
	}
	copied := *config
	copied.Labels = maps.Clone(config.Labels)
	if copied.Labels == nil {
		copied.Labels = map[string]string{}
	}
	copied.Labels["io.docklet.run"] = run
	if len(old.ID) >= 12 && copied.Hostname == old.ID[:12] {
		copied.Hostname = ""
	}
	created, err := compat.ContainerCreateWithCompatibilityForAPIVersion(ctx, c, client.ContainerCreateOptions{Config: &copied, HostConfig: host, NetworkingConfig: networks, Name: name}, apiVersion)
	if err != nil {
		return fail("create", err)
	}
	j.NewID = created.ID
	if err := s.saveRecreate(ctx, j, "created"); err != nil {
		return "", err
	}
	if j.WasRunning {
		if err := s.saveRecreate(ctx, j, "intent-start"); err != nil {
			return "", err
		}
		if j.AutoRemove {
			if _, err := c.ContainerStop(ctx, j.OldID, client.ContainerStopOptions{Timeout: new(30)}); err != nil {
				return fail("stop", err)
			}
			if err := s.saveRecreate(ctx, j, "intent-start"); err != nil {
				return "", err
			}
		}
		if _, err := c.ContainerStart(ctx, j.NewID, client.ContainerStartOptions{}); err != nil {
			return fail("start", err)
		}
		if err := s.saveRecreate(ctx, j, "started"); err != nil {
			return "", err
		}
	}
	if err := s.saveRecreate(ctx, j, "intent-remove-old"); err != nil {
		return "", err
	}
	if !j.AutoRemove || !j.WasRunning {
		if _, err := c.ContainerRemove(ctx, j.OldID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return fail("remove", err)
		}
	}
	if err := s.saveRecreate(ctx, j, "done"); err != nil {
		return "", err
	}
	result := recreateResult(j)
	dto.RecordRecreateResult(ctx, *result)
	return j.NewID, nil
}
func recreateResult(j *RecreateJournal) *dto.RecreateResult {
	return &dto.RecreateResult{OldID: j.OldID, NewID: j.NewID, OldName: j.OldName, NewName: j.NewName}
}
func recreateInspect(ctx context.Context, c *client.Client, id string) (*dockercontainer.InspectResponse, error) {
	if id == "" {
		return nil, nil
	}
	response, err := compat.ContainerInspectWithCompatibility(ctx, c, id, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &response.Container, nil
}
func (s *ContainerService) ambiguousRecreate(ctx context.Context, j *RecreateJournal) (dto.RecreateRecovery, error) {
	err := s.saveRecreate(ctx, j, "ambiguous")
	return dto.RecreateRecovery{RunID: j.RunID, Code: "container.recreate-ambiguous"}, err
}
func (s *ContainerService) recoveredRecreate(ctx context.Context, j *RecreateJournal) (dto.RecreateRecovery, error) {
	err := s.saveRecreate(ctx, j, "rolled-back")
	return dto.RecreateRecovery{RunID: j.RunID, Code: "container.recreate-recovered"}, err
}

// Compensation persists each intent, observes exact IDs and tolerates an effect
// already completed by the prior process. It never recreates or pulls an image.
func (s *ContainerService) compensateRecreate(ctx context.Context, c *client.Client, j *RecreateJournal, old, replacement *dockercontainer.InspectResponse) (dto.RecreateRecovery, error) {
	if old == nil {
		return s.ambiguousRecreate(ctx, j)
	}
	if replacement != nil {
		if err := s.saveRecreate(ctx, j, "comp-remove-new"); err != nil {
			return dto.RecreateRecovery{}, err
		}
		if _, err := c.ContainerRemove(ctx, replacement.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return s.ambiguousRecreate(ctx, j)
		}
		if err := s.saveRecreate(ctx, j, "comp-rename-old"); err != nil {
			return dto.RecreateRecovery{}, err
		}
	}
	if strings.TrimPrefix(old.Name, "/") != j.OldName {
		claimed, err := recreateInspect(ctx, c, j.OldName)
		if err != nil {
			return dto.RecreateRecovery{}, err
		}
		if claimed != nil && claimed.ID != j.OldID {
			return s.ambiguousRecreate(ctx, j)
		}
		if err := s.saveRecreate(ctx, j, "comp-rename-old"); err != nil {
			return dto.RecreateRecovery{}, err
		}
		if _, err := c.ContainerRename(ctx, j.OldID, client.ContainerRenameOptions{NewName: j.OldName}); err != nil {
			return s.ambiguousRecreate(ctx, j)
		}
		if err := s.saveRecreate(ctx, j, "comp-start-old"); err != nil {
			return dto.RecreateRecovery{}, err
		}
	}
	if j.WasRunning && (old.State == nil || !old.State.Running) {
		if err := s.saveRecreate(ctx, j, "comp-start-old"); err != nil {
			return dto.RecreateRecovery{}, err
		}
		if _, err := c.ContainerStart(ctx, j.OldID, client.ContainerStartOptions{}); err != nil {
			return s.ambiguousRecreate(ctx, j)
		}
		if err := s.saveRecreate(ctx, j, "rolled-back"); err != nil {
			return dto.RecreateRecovery{}, err
		}
	}
	return s.recoveredRecreate(ctx, j)
}
func (s *ContainerService) finishRecoveredRecreate(ctx context.Context, c *client.Client, j *RecreateJournal, old *dockercontainer.InspectResponse) (dto.RecreateRecovery, error) {
	if err := s.saveRecreate(ctx, j, "intent-remove-old"); err != nil {
		return dto.RecreateRecovery{}, err
	}
	if old != nil {
		if _, err := c.ContainerRemove(ctx, j.OldID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return s.ambiguousRecreate(ctx, j)
		}
	}
	if err := s.saveRecreate(ctx, j, "done"); err != nil {
		return dto.RecreateRecovery{}, err
	}
	return dto.RecreateRecovery{RunID: j.RunID, Result: recreateResult(j)}, nil
}
func (s *ContainerService) recoverRecreate(ctx context.Context, c *client.Client, j *RecreateJournal) (dto.RecreateRecovery, error) {
	if j.Phase == "ambiguous" {
		return dto.RecreateRecovery{RunID: j.RunID, Code: "container.recreate-ambiguous"}, nil
	}
	old, err := recreateInspect(ctx, c, j.OldID)
	if err != nil {
		return dto.RecreateRecovery{}, err
	}
	replacement, err := recreateInspect(ctx, c, j.NewID)
	if err != nil {
		return dto.RecreateRecovery{}, err
	}
	if j.NewID == "" && (j.Phase == "intent-create" || j.Phase == "created") {
		candidate, err := recreateInspect(ctx, c, j.NewName)
		if err != nil {
			return dto.RecreateRecovery{}, err
		}
		if candidate != nil {
			created, parseErr := time.Parse(time.RFC3339Nano, candidate.Created)
			if candidate.Config == nil || candidate.Config.Labels["io.docklet.run"] != j.RunID || parseErr != nil || created.Before(j.UpdatedAt) {
				return s.ambiguousRecreate(ctx, j)
			}
			replacement = candidate
			j.NewID = candidate.ID
			// Preserve the phase: finding a candidate is evidence, never a create replay.
			if err := s.saveRecreate(ctx, j, j.Phase); err != nil {
				return dto.RecreateRecovery{}, err
			}
		}
	}
	// Missing IDs take precedence over forward/compensation state decisions.
	if old == nil && replacement == nil {
		return s.ambiguousRecreate(ctx, j)
	}
	// The original is expected to be gone only after removal began, or after an
	// AutoRemove original was stopped before the replacement started.
	if old == nil && j.Phase != "intent-remove-old" && j.Phase != "done" && (!j.AutoRemove || j.Phase != "started") {
		return s.ambiguousRecreate(ctx, j)
	}
	if old != nil && (old.ID != j.OldID || (strings.TrimPrefix(old.Name, "/") != j.OldName && strings.TrimPrefix(old.Name, "/") != j.BackupName)) {
		return s.ambiguousRecreate(ctx, j)
	}
	if replacement != nil && (replacement.ID != j.NewID || strings.TrimPrefix(replacement.Name, "/") != j.NewName || replacement.Config == nil || replacement.Config.Labels["io.docklet.run"] != j.RunID) {
		return s.ambiguousRecreate(ctx, j)
	}
	// Every reserved name belongs only to these exact IDs. A foreign claim
	// is ambiguous even when the expected replacement itself still exists.
	for _, name := range []string{j.OldName, j.BackupName, j.NewName} {
		observed, err := recreateInspect(ctx, c, name)
		if err != nil {
			return dto.RecreateRecovery{}, err
		}
		if observed != nil && observed.ID != j.OldID && observed.ID != j.NewID {
			return s.ambiguousRecreate(ctx, j)
		}
	}
	if strings.HasPrefix(j.Phase, "comp-") {
		return s.compensateRecreate(ctx, c, j, old, replacement)
	}
	if j.Phase == "rolled-back" {
		return dto.RecreateRecovery{RunID: j.RunID, Code: "container.recreate-recovered"}, nil
	}
	if replacement == nil {
		if j.NewID != "" {
			return s.ambiguousRecreate(ctx, j)
		}
		if j.Phase == "intent-rename" && old != nil && strings.TrimPrefix(old.Name, "/") == j.OldName {
			return s.recoveredRecreate(ctx, j)
		}
		return s.compensateRecreate(ctx, c, j, old, nil)
	}
	if !j.WasRunning {
		return s.finishRecoveredRecreate(ctx, c, j, old)
	}
	// After the observed start the replacement is kept as observed, even when a
	// host or daemon restart stopped it. Only intent-start needs a running proof.
	if j.Phase == "started" || j.Phase == "intent-remove-old" || j.Phase == "done" {
		return s.finishRecoveredRecreate(ctx, c, j, old)
	}
	if j.Phase == "intent-start" && replacement.State != nil && replacement.State.Running {
		return s.finishRecoveredRecreate(ctx, c, j, old)
	}
	return s.compensateRecreate(ctx, c, j, old, replacement)
}

// RecoverRecreates runs before generic startup settlement and admission. Only
// unresolved local receipts are recovered; a write failure leaves them excluded.
func (s *ContainerService) RecoverRecreates(ctx context.Context) ([]dto.RecreateRecovery, error) {
	if s.operations == nil || s.operations.db == nil {
		return nil, nil
	}
	var journals []RecreateJournal
	err := s.operations.db.WithContext(ctx).Table("container_recreate_journal").Where("run_id IN (SELECT run_id FROM runtime_receipts WHERE environment_id='0' AND settled_at IS NULL)").Order("run_id").Find(&journals).Error
	if err != nil {
		return nil, err
	}
	if len(journals) == 0 {
		return nil, nil
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]dto.RecreateRecovery, 0, len(journals))
	for i := range journals {
		result, err := s.recoverRecreate(ctx, c, &journals[i])
		if err != nil {
			return nil, fmt.Errorf("recreate recovery: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}
func (s *ContainerService) PruneRecreateJournals(ctx context.Context) error {
	if s.operations == nil || s.operations.db == nil {
		return nil
	}
	return s.operations.db.WithContext(ctx).Where("phase IN ('done','rolled-back') AND updated_at<? AND run_id IN (SELECT run_id FROM runtime_receipts WHERE settled_at IS NOT NULL)", time.Now().UTC().Add(-7*24*time.Hour)).Delete(&RecreateJournal{}).Error
}
