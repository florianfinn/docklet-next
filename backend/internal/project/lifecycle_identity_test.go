// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Store-aware execution, local evidence and replica guard proofs.

package project

import (
	"context"
	"encoding/json"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	"maps"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	a "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIdentityLifecycleStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd", "containerd-arm"} {
		t.Run(store, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.containerd = store != "classic"
			wantedPlatform := "linux/amd64"
			if store == "containerd-arm" {
				wantedPlatform = "linux/arm/v7"
				f.platform = wantedPlatform
			}
			baseline := f.establishBaseline()
			f.up = func(_ context.Context, model *compose.Project) error {
				if model.Services["app"].Platform != wantedPlatform {
					return errors.New("incorrect execution platform")
				}
				return nil
			}
			if !f.containerd {
				require.Empty(t, baseline.Services["app"].Digest)
			} else {
				require.NotEqual(t, baseline.Services["app"].ImageID, baseline.Services["app"].Digest)
			}
			old := lifecycleFile(t, f, "compose.yaml")
			intent := f.intent("rebuild")
			expected := intent.Selections[0].ImageID
			updated := f.wait(f.admit(intent))
			require.Equal(t, a.StatusSuccess, updated.Status, "%+v", updated.ImageLifecycle)
			require.True(t, updated.ImageLifecycle.Verified)
			require.Equal(t, []string{wantedPlatform}, f.pullPlatforms)
			require.Equal(t, old, lifecycleFile(t, f, "compose.yaml"))
			_, current, e := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, e)
			require.Equal(t, expected, current.Services["app"].ImageID)
			if f.containerd {
				require.Equal(t, expected, current.Services["app"].Digest)
			} else {
				require.Empty(t, current.Services["app"].Digest)
			}
			points := lifecyclePoints(t, f)
			require.Len(t, points, 1)
			// Restore and local baseline observation need no registry service.
			f.service.containerRegistryService = nil
			restored := f.wait(f.admit(lifecycleRestore(t, f, points[0].ID)))
			require.Equal(t, a.StatusSuccess, restored.Status, "%+v", restored.ImageLifecycle)
			_, current, e = f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, e)
			require.Equal(t, lifecycleOldID, current.Services["app"].ImageID)
		})
	}
}
func TestIdentityNoOpSelection(t *testing.T) {
	f := newLifecycleFixture(t)
	snap := f.establishBaseline()
	sel := f.intent("rebuild").Selections[0]
	sel.ImageID = lifecycleOldID
	require.True(t, lifecycleSelectionCurrent(snap, sel))
	for _, shape := range []string{"absent", "oneoff", "different", "missing-replica"} {
		t.Run(shape, func(t *testing.T) {
			copy := *snap
			copy.Containers = map[string]life.ServiceImage{}
			for key, value := range snap.Containers {
				copy.Containers[key] = value
			}
			switch shape {
			case "absent":
				clear(copy.Containers)
			case "oneoff":
				for key, v := range copy.Containers {
					v.OneOff = true
					copy.Containers[key] = v
				}
			case "different":
				copy.Containers["second"] = life.ServiceImage{Service: "app", ImageID: lifecycleNewID}
			case "missing-replica":
				model := *copy.Model
				model.Services = maps.Clone(copy.Model.Services)
				svc := model.Services["app"]
				replicas := 2
				svc.Deploy = &compose.DeployConfig{Replicas: &replicas}
				model.Services["app"] = svc
				copy.Model = &model
			}
			require.False(t, lifecycleSelectionCurrent(&copy, sel))
		})
	}
	intent := f.intent("rebuild")
	intent.Selections[0].ImageID = lifecycleOldID
	result, e := f.service.ExecuteImageLifecycle(t.Context(), f.project.ID, uuid.NewString(), f.registration, intent, user.Actor{ID: "operator"})
	require.Error(t, e)
	require.Equal(t, "job.skipped", result.Code)
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
	require.Empty(t, lifecyclePoints(t, f))
}
func TestIdentityMissingIDRejected(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		t.Run(store, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.containerd = store == "containerd"
			f.establishBaseline()
			intent := f.intent("rebuild")
			intent.Selections[0].ImageID = ""
			_, e := f.runtime.Admit(t.Context(), activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator"}, Operation: activity.RuntimeUpdateProject, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID}, Options: activity.RuntimeOptions{Lifecycle: intent}})
			require.Error(t, e)
			// A corrupt frozen receipt is still rejected after serialization/restart;
			// neither an empty ID nor a concurrent tag mover can cause rediscovery.
			body, e := json.Marshal(intent)
			require.NoError(t, e)
			var reopened dto.LifecycleIntent
			require.NoError(t, json.Unmarshal(body, &reopened))
			f.onInspect = func(ref string) {
				if ref == lifecycleOldID {
					f.mu.Lock()
					f.tags["registry.example.com/app:1.0.0"] = lifecycleNewID
					f.mu.Unlock()
				}
			}
			result, e := f.service.ExecuteImageLifecycle(context.Background(), f.project.ID, uuid.NewString(), f.registration, &reopened, user.Actor{ID: "operator"})
			require.Error(t, e)
			require.False(t, result.Verified)
			require.Zero(t, f.pulls)
			require.Zero(t, f.deploys)
			require.Empty(t, lifecyclePoints(t, f))
			var receipts int64
			require.NoError(t, f.db.Table("runtime_receipts").Count(&receipts).Error)
			require.Zero(t, receipts)
		})
	}
}
func TestIdentityLocalEvidenceStores(t *testing.T) {
	for _, containerd := range []bool{false, true} {
		t.Run(strings.TrimSpace(map[bool]string{true: "containerd", false: "classic"}[containerd]), func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.containerd = containerd
			snap := f.establishBaseline()
			before := lifecycleObserved(snap)
			if !containerd {
				img := snap.Services["app"]
				img.Digest = lifecycleDigest
				snap.Services["app"] = img
			}
			f.service.containerRegistryService = nil
			require.NoError(t, f.service.lifecycleObserve(t.Context(), snap))
			require.Equal(t, before, lifecycleObserved(snap))
			if !containerd {
				require.Empty(t, snap.Services["app"].Digest)
			}
		})
	}
}

func TestUnsetBaselineRead(t *testing.T) {
	f := newLifecycleFixture(t)
	status, e := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
	require.NoError(t, e)
	require.True(t, status.Valid())
	require.False(t, status.Available)
	require.Equal(t, "update.baseline-unavailable", status.Code)
	require.Empty(t, status.SourceRunID)
	points, e := f.service.ListImageRestorePoints(t.Context(), f.project.ID, false)
	require.NoError(t, e)
	require.NotNil(t, points)
	require.Empty(t, points)
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
}
