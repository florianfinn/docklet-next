// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-source update, failure, restore and instance regressions.

package container

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	dto "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/cgroup"
)

func TestContainerImageNoCandidate(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	p.checked.HasUpdate = false
	ctx := updateContext(t)
	var result updaterDTO.LifecycleResult
	ctx = updaterDTO.WithResultSink(ctx, func(r updaterDTO.LifecycleResult) { result = r })
	require.Equal(t, "job.skipped", imageDTO.Neutral(s.ExecuteContainerImage(ctx, updateOldID, "app", false, user.Actor{}), "unexpected", 500).Problem.Code)
	require.Equal(t, "job.skipped", result.Code)
	require.Empty(t, d.counts)
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Nil(t, slot)
}
func TestContainerImageOriginExecutor(t *testing.T) {
	for _, restore := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			labels map[string]string
		}{
			{"unraid", map[string]string{"net.unraid.docker.managed": ""}},
			{"compose", map[string]string{"com.docker.compose.project": "stack", "com.docker.compose.service": "app"}},
			{"self", map[string]string{"com.getarcaneapp.arcane": "true"}},
		} {
			t.Run(tc.name+map[bool]string{false: "/update", true: "/restore"}[restore], func(t *testing.T) {
				s, d, _, _ := updateFixture(t)
				old := d.containers[updateOldID]
				old.Config.Labels = tc.labels
				id := updateOldID
				if tc.name == "self" {
					current, err := cgroup.CurrentContainerID()
					if err == nil && current != "" {
						id = current
						old.ID = id
						delete(d.containers, updateOldID)
					}
				}
				d.containers[id] = old
				err := s.ExecuteContainerImage(updateContext(t), id, "app", restore, user.Actor{})
				var p *dto.Problem
				require.ErrorAs(t, err, &p)
				require.Equal(t, "container.origin-locked", p.Code)
				require.Empty(t, d.counts)
			})
		}
	}
}
func TestContainerImageFailuresReplaceSlotBeforeRecreate(t *testing.T) {
	for _, failure := range []string{"pull", "digest", "create", "start"} {
		t.Run(failure, func(t *testing.T) {
			s, d, p, _ := updateFixture(t)
			require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, "registry.example.com/app:older", updateOldID, "older-run"))
			if failure == "digest" {
				d.moved = true
			} else {
				d.fail = failure
			}
			d.createSlot = func() {
				slot, err := s.previousImage(t.Context(), "app")
				require.NoError(t, err)
				require.Equal(t, updateOldImage, slot.ImageID)
				require.Equal(t, updateOldID, slot.OwnerContainerID)
			}
			ctx := updateContext(t)
			err := s.ExecuteContainerImage(ctx, updateOldID, "app", false, user.Actor{})
			code := "update.deploy-failed"
			if failure == "pull" {
				code = "image.pull-failed"
			}
			if failure == "digest" {
				code = "update.target-changed"
			}
			require.Equal(t, code, imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
			slot, e := s.previousImage(t.Context(), "app")
			require.NoError(t, e)
			if failure == "pull" || failure == "digest" {
				require.Equal(t, updateOtherImage, slot.ImageID)
				require.Zero(t, d.counts["rename"])
				require.Zero(t, d.counts["create"])
			} else {
				require.Equal(t, updateOldImage, slot.ImageID)
				require.Equal(t, updateOldID, slot.OwnerContainerID)
				old := d.containers[updateOldID]
				require.Equal(t, "/app", old.Name)
				require.True(t, old.State.Running)
				details := dto.Details{ID: updateOldID, Name: "app", ImageID: updateOldImage}
				require.NoError(t, s.projectPreviousImage(t.Context(), &details))
				require.Nil(t, details.PreviousImage)
				reasons, e := p.g.Reasons(t.Context(), updateOtherImage)
				require.NoError(t, e)
				require.Empty(t, reasons)
			}
		})
	}
}
func TestContainerImageUpdateRestoreSameTagAndConfiguration(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	ctx := updateContext(t)
	require.NoError(t, s.ExecuteContainerImage(ctx, updateOldID, "app", false, user.Actor{}))
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateNewID, slot.OwnerContainerID)
	require.Equal(t, updaterDTO.ExecutionIdentityFromContext(ctx).RunID, slot.RunID)
	details := dto.Details{ID: updateNewID, Name: "app", ImageID: updateNewImage}
	require.NoError(t, s.projectPreviousImage(t.Context(), &details))
	require.NotNil(t, details.PreviousImage)
	require.Equal(t, updateOldImage, details.PreviousImage.ImageID)
	report, e := p.g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.NoError(t, e, "%+v", imageDTO.Neutral(e, "unexpected", 500))
	require.Equal(t, []imageDTO.PruneItem{{ID: updateOldImage, Code: "image.restore-point-protected"}}, report.Skipped)
	require.Empty(t, report.ImagesDeleted)
	require.Zero(t, d.counts["delete-image"])
	reasons, err := p.g.Reasons(t.Context(), updateOldImage)
	require.NoError(t, err)
	require.Equal(t, []imageDTO.Protection{{Reason: "restore-point", OwnerID: "app"}}, reasons)
	replacement := d.containers[updateNewID]
	require.Equal(t, []string{"EXAMPLE=kept"}, replacement.Config.Env)
	require.Equal(t, []string{"data:/data"}, replacement.HostConfig.Binds)
	require.Equal(t, 1, d.counts["pull"])
	require.Zero(t, d.counts["tag"])
	require.Equal(t, 1, d.counts["create"])
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateNewID, "app", true, user.Actor{}))
	require.Equal(t, 1, d.counts["tag"])
	require.Equal(t, updateOldImage, d.containers[updateRestoreID].Image)
	require.Equal(t, updateRef, d.containers[updateRestoreID].Config.Image)
	require.Equal(t, 1, d.counts["pull"], "restore must not pull")
}
func TestContainerImageStaleSlotAndFrozenInstance(t *testing.T) {
	for _, tc := range []string{"outside-slot", "other-recreate", "admission-instance", "renamed", "same-image"} {
		t.Run(tc, func(t *testing.T) {
			s, d, _, _ := updateFixture(t)
			owner := updateNewID
			name := "app"
			restore := true
			if tc == "admission-instance" {
				restore = false
			}
			if tc == "renamed" {
				name = "old-app"
				restore = false
			}
			slotImage := updateOtherImage
			if tc == "same-image" {
				// The slot already holds the running image: nothing to restore.
				owner, slotImage = updateOldID, updateOldImage
			}
			require.NoError(t, s.replacePreviousImage(t.Context(), "app", slotImage, updateRef, owner, "old-run"))
			details := dto.Details{ID: updateOldID, Name: "app", ImageID: updateOldImage}
			require.NoError(t, s.projectPreviousImage(t.Context(), &details))
			require.Nil(t, details.PreviousImage)
			id := updateOldID
			if tc == "admission-instance" {
				id = updateNewID
			}
			err := s.ExecuteContainerImage(updateContext(t), id, name, restore, user.Actor{})
			require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
			require.Empty(t, d.counts)
		})
	}
}
func TestContainerImageInstanceChangesDuringCheck(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	p.check = func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		old := d.containers[updateOldID]
		old.ID = updateNewID
		d.containers[updateNewID] = old
		delete(d.containers, updateOldID)
	}
	err := s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Empty(t, d.counts, "identity changes during a slow check must precede image effects")
}
func TestContainerImageMovedTagAfterPullLeavesReplacementAndStaleOwner(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	d.createSlot = func() { d.tag = updateOtherImage }
	err := s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	slot, e := s.previousImage(t.Context(), "app")
	require.NoError(t, e)
	require.Equal(t, updateOldID, slot.OwnerContainerID)
	require.Equal(t, updateOtherImage, d.containers[updateNewID].Image)
	require.NotContains(t, d.containers, updateOldID)
}
func TestContainerSlotAdvanceIsConditional(t *testing.T) {
	s, _, _, _ := updateFixture(t)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOldImage, updateRef, updateOldID, "newer-run"))
	require.NoError(t, s.advancePreviousImage(context.Background(), &RecreateJournal{RunID: "older-run", OldID: updateOldID, OldName: "app", NewID: updateNewID}))
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateOldID, slot.OwnerContainerID)
	require.NoError(t, s.advancePreviousImage(t.Context(), &RecreateJournal{RunID: "newer-run", OldID: strings.Repeat("e", 64), OldName: "app", NewID: updateNewID}))
	slot, err = s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateOldID, slot.OwnerContainerID)
}

func TestContainerImageRecreateKeepsNamePolicy(t *testing.T) {
	s, d, _, db := updateFixture(t)
	store := policies.Store{DB: db.DB}
	p := policies.Policy{ID: "saved-policy", EnvironmentID: "0", RegistrationID: "registration", TargetKind: "container", TargetID: "app", TargetIdentity: "app", Mode: "scheduled", Revision: 1, ActorID: "operator", Candidates: "[]"}
	p.SetWindow(&updaterDTO.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, db.Create(&p).Error)
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	replacement := d.containers[updateNewID]
	observed, err := store.Policy(t.Context(), "0", "registration", strings.TrimPrefix(replacement.Name, "/"), "container")
	require.NoError(t, err)
	require.Equal(t, "scheduled", observed.Mode)
	require.Equal(t, "saved-policy", observed.ID)
	require.Equal(t, "app", observed.TargetID)
	byID, err := store.Policy(t.Context(), "0", "registration", updateNewID, "container")
	require.NoError(t, err)
	require.Equal(t, "check", byID.Mode)
}

func TestContainerSlotPinTransactionRollback(t *testing.T) {
	s, d, p, db := updateFixture(t)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, updateRef, updateOldID, "older-run"))
	require.NoError(t, db.Exec("CREATE TRIGGER fixture_slot_refusal BEFORE UPDATE ON container_previous_images BEGIN SELECT RAISE(ABORT,'synthetic slot refusal'); END").Error)
	require.Error(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Zero(t, d.counts["rename"])
	require.Zero(t, d.counts["create"])
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateOtherImage, slot.ImageID)
	require.Equal(t, "older-run", slot.RunID)
	oldPins, err := p.g.Reasons(t.Context(), updateOtherImage)
	require.NoError(t, err)
	require.Len(t, oldPins, 1)
	newPins, err := p.g.Reasons(t.Context(), updateOldImage)
	require.NoError(t, err)
	require.Empty(t, newPins)
}

func TestContainerImageVersionReferenceAndPlatform(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	p.checked.State = "version"
	p.checked.CurrentVersion = "1.0.0"
	p.checked.LatestVersion = "1.1.0"
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Equal(t, "registry.example.com/app:1.1.0", d.containers[updateNewID].Config.Image)
	require.Equal(t, "registry.example.com/app", d.pullRef)
	require.Equal(t, "linux/amd64", d.pullPlatform)
}

// A rebuild whose newer version label differs keeps the configured reference.
func TestContainerImageRebuildKeepsReference(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	p.checked.CurrentVersion = "1.2.3-ls10"
	p.checked.LatestVersion = "1.2.3-ls11"
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Equal(t, updateRef, d.containers[updateNewID].Config.Image)
}
func TestContainerImageChangedCandidateBeforePull(t *testing.T) {
	s, d, p, _ := updateFixture(t)
	p.checked.LatestDigest = updateOtherImage
	err := s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Empty(t, d.counts)
}

func TestContainerImageMissingServiceIsUnavailable(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	unconfigured := NewContainerService(nil, s.dockerService, nil, nil, nil)
	err := unconfigured.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.check-unavailable", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Empty(t, d.counts)
}

func TestContainerPreviousImageAfterUnrelatedDockletRecreate(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, updateRef, updateOldID, "earlier-update"))
	replacement, err := s.EditContainer(updateContext(t), updateOldID, dto.Edit{}, user.Actor{})
	require.NoError(t, err)
	require.Equal(t, updateNewID, replacement)
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateOldID, slot.OwnerContainerID, "an ordinary edit must not advance the update slot")
	details := dto.Details{ID: replacement, Name: "app", ImageID: updateOldImage}
	require.NoError(t, s.projectPreviousImage(t.Context(), &details))
	require.Nil(t, details.PreviousImage)
	before := maps.Clone(d.counts)
	err = s.ExecuteContainerImage(updateContext(t), replacement, "app", true, user.Actor{})
	require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Equal(t, before, d.counts)
}

func TestContainerImageRestoreIdentityChangesDuringInspect(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, updateRef, updateOldID, "previous-run"))
	d.beforeRefInspect = func() {
		old := d.containers[updateOldID]
		old.ID = updateNewID
		d.containers[updateNewID] = old
		delete(d.containers, updateOldID)
	}
	err := s.ExecuteContainerImage(updateContext(t), updateOldID, "app", true, user.Actor{})
	require.Equal(t, "update.target-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Empty(t, d.counts, "identity changes during a slow inspect must precede retag")
}

// The restore offer is optional: a failing slot read hides it and still returns the details.
func TestContainerDetailsSurviveSlotReadFailure(t *testing.T) {
	s, _, _, db := updateFixture(t)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, updateRef, updateOldID, "old-run"))
	require.NoError(t, db.Exec("DROP TABLE container_previous_images").Error)
	details, err := s.GetContainerDetails(updateContext(t), updateOldID)
	require.NoError(t, err)
	require.Equal(t, updateOldID, details.ID)
	require.Nil(t, details.PreviousImage)
}

// Under the containerd store the pulled image ID names the index; the
// candidate names the platform manifest, and the container runs the index ID.
func TestContainerImageUpdateComparesNativeKeyOnContainerdStore(t *testing.T) {
	s, d, p, _ := updateFixtureAPI(t, "1.49")
	manifest := "sha256:" + strings.Repeat("9", 64)
	d.containerd = map[string]string{updateNewImage: manifest}
	p.expected, p.checked.LatestDigest = manifest, manifest
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	require.Equal(t, updateNewImage, d.containers[updateNewID].Image)
	require.Equal(t, 1, d.counts["create"])
}
