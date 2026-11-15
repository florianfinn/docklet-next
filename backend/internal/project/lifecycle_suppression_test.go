// Source: docklet-next/issues/440
// Revision: 92ca1a3ea43cfdc211ca561a968e09c8e8f747b1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original scheduled manifest suppression and manual path controls through real runtime dispatch.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/stability/stabilitytest"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestObservedStackSuppressionUsesPullableManifest(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		for _, which := range []string{"manifest", "native", "other-service", "other-platform", "other-repository"} {
			t.Run(store+"/"+which, func(t *testing.T) {
				f, _ := observedStackFixture(t)
				if store == "containerd" {
					f.containerd = true
					f.establishBaseline()
				}
				intent := observedStackIntent(t, f, "scheduled", "version")
				tuple := dto.SuppressedCandidate{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: lifecycleDigest}
				switch which {
				case "native":
					tuple.Manifest = lifecycleNewID
				case "other-service":
					tuple.Service = "other"
				case "other-platform":
					tuple.Platform = "linux/arm64"
				case "other-repository":
					tuple.Repository = "registry.example.com/other"
				}
				intent.Policy.Suppressed = []dto.SuppressedCandidate{tuple}
				out := f.wait(f.admit(intent))
				if which == "manifest" {
					require.Equal(t, activityDTO.StatusFailed, out.Status)
					require.Equal(t, "update.candidate-failed", out.ImageLifecycle.Code)
					require.Equal(t, &dto.RecoveryResult{Outcome: "not-needed"}, out.ImageLifecycle.Recovery)
					require.Empty(t, out.ImageLifecycle.RestorePointID)
					require.Empty(t, out.ImageLifecycle.Applied)
					require.Zero(t, f.pulls)
					require.Zero(t, f.tagWrites)
					require.Zero(t, f.deploys)
					require.Empty(t, lifecyclePoints(t, f))
				} else {
					require.Equal(t, activityDTO.StatusSuccess, out.Status, "%+v", out.ImageLifecycle)
					require.Len(t, out.ImageLifecycle.Applied, 1)
					require.Equal(t, lifecycleDigest, out.ImageLifecycle.Applied[0].Manifest)
					require.Equal(t, 1, f.pulls)
				}
			})
		}
	}
}
func TestObservedStackMixedSuppressionRetainsExistingGuards(t *testing.T) {
	for _, shape := range []string{"independent", "namespace-coupled", "shared-source", "rebuild-shared"} {
		t.Run(shape, func(t *testing.T) {
			source := serviceScopeSource
			if shape == "namespace-coupled" {
				source = strings.Replace(source, "    network_mode: none", "    network_mode: service:running", 1)
			}
			if shape == "shared-source" {
				source = "services:\n  app:\n    image: &shared registry.example.com/app:1.0.0\n  running:\n    image: *shared\n"
			}
			if shape == "rebuild-shared" {
				source = strings.Replace(source, "registry.example.com/side:1.0.0", "registry.example.com/app:1.0.0", -1)
			}
			f := serviceScopeFixture(t, source)
			if shape == "shared-source" || shape == "rebuild-shared" {
				c := f.workloads["running"]
				c.Reference = "registry.example.com/app:1.0.0"
				f.workloads["running"] = c
			}
			f.establishBaseline()
			f.service.UseObservationClock(stabilitytest.NewClock())
			intent := observedStackIntent(t, f, "scheduled", "version")
			reference := "registry.example.com/side:1.1.0"
			sourceRef := "registry.example.com/side:1.0.0"
			kind := "version"
			if shape == "shared-source" || shape == "rebuild-shared" {
				sourceRef = "registry.example.com/app:1.0.0"
				reference = "registry.example.com/app:1.1.0"
			}
			if shape == "rebuild-shared" {
				kind = "rebuild"
				reference = sourceRef
			}
			intent.Selections = append(intent.Selections, dto.ImageSelection{Service: "running", SourceReference: sourceRef, Reference: reference, Digest: lifecycleDigest, ImageID: lifecycleNewID, Platform: "linux/amd64", Kind: kind})
			intent.Policy.Suppressed = []dto.SuppressedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: lifecycleDigest}}
			f.up = func(_ context.Context, model *compose.Project) error {
				require.Len(t, model.Services, 1)
				require.Contains(t, model.Services, "running")
				return nil
			}
			out := f.wait(f.admit(intent))
			if shape == "independent" {
				require.Equal(t, activityDTO.StatusSuccess, out.Status, "%+v", out.ImageLifecycle)
				require.Equal(t, lifecycleOldID, f.workloads["app"].ImageID)
				require.Equal(t, lifecycleNewID, f.workloads["running"].ImageID)
				require.Len(t, out.ImageLifecycle.Applied, 1)
				require.Equal(t, "running", out.ImageLifecycle.Applied[0].Service)
				require.Equal(t, 1, f.pulls)
			} else {
				code := "update.service-coupled"
				if shape == "shared-source" {
					code = "update.image-binding-unsupported"
				}
				require.Equal(t, activityDTO.StatusFailed, out.Status)
				require.Equal(t, code, out.ImageLifecycle.Code)
				require.Zero(t, f.pulls)
				require.Zero(t, f.tagWrites)
				require.Zero(t, f.deploys)
			}
		})
	}
}
func TestObservedStackManualModeHasNoSuppressionOrApplied(t *testing.T) {
	f, clock := observedStackFixture(t)
	intent := f.intent("version")
	intent.Request.AutoRestore = true
	ctx := dto.WithObservedExecution(t.Context(), dto.ObservedExecution{Mode: "manual", Suppressed: []dto.SuppressedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: lifecycleDigest}}})
	result, err := f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, intent, user.Actor{ID: "operator", Username: "operator"})
	require.NoError(t, err)
	require.True(t, result.Verified)
	require.Empty(t, result.Applied)
	require.Positive(t, clock.waits)
	require.Equal(t, 1, f.pulls)
}
func TestObservedStackPlainManualKeepsExistingResult(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			f, clock := observedStackFixture(t)
			f.up = func(context.Context, *compose.Project) error {
				if fail {
					return errors.New("synthetic deploy failure")
				}
				return nil
			}
			out := f.wait(f.admit(f.intent("version")))
			want := &dto.LifecycleResult{RunID: out.ID, Verified: !fail, RestorePointID: out.ImageLifecycle.RestorePointID, VolumeDataIncluded: false}
			if fail {
				want.Code = "update.deploy-failed"
				want.HTTPStatus = 500
			}
			require.Equal(t, want, out.ImageLifecycle, "plain manual evidence keeps its existing full shape")
			require.Zero(t, clock.waits)
			require.Equal(t, 1, f.deploys)
			if fail {
				require.Contains(t, lifecycleFile(t, f, "compose.yaml"), "app:1.1.0")
			}
		})
	}
}
func TestObservedStackRestoredBaselineAllowsNextSuppressedAttempt(t *testing.T) {
	f, _ := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			return errors.New("synthetic deploy failure")
		}
		return nil
	}
	first := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	requireStackRecovery(t, first, "restored", "")
	next := observedStackIntent(t, f, "scheduled", "version")
	next.Policy.Suppressed = first.ImageLifecycle.Applied
	second := f.wait(f.admit(next))
	require.Equal(t, "update.candidate-failed", second.ImageLifecycle.Code)
	require.Equal(t, 2, f.deploys)
	require.Equal(t, 1, f.pulls)
}
func TestObservedStackEvidenceStaysWithinCheckedBound(t *testing.T) {
	f, _ := observedStackFixture(t)
	intent := observedStackIntent(t, f, "scheduled", "version")
	out := f.wait(f.admit(intent))
	body, err := json.Marshal(out.ImageLifecycle)
	require.NoError(t, err)
	require.Less(t, len(body), 2048)
	require.Len(t, out.ImageLifecycle.Applied, 1)
	// Refuse an oversized applied set before preparing points or any engine effect.
	g, _ := observedStackFixture(t)
	tooMany := observedStackIntent(t, g, "scheduled", "version")
	for n := 0; n < dto.MaxAppliedCandidates; n++ {
		tooMany.Selections = append(tooMany.Selections, tooMany.Selections[0])
	}
	ctx := dto.WithObservedExecution(t.Context(), dto.ObservedExecution{Mode: "scheduled"})
	result, err := g.service.ExecuteImageLifecycle(ctx, g.project.ID, uuid.NewString(), g.registration, tooMany, user.Actor{ID: "operator", Username: "operator"})
	lifecycleCode(t, err, "update.invalid-target")
	require.Empty(t, result.RestorePointID)
	require.Zero(t, g.pulls)
	require.Empty(t, lifecyclePoints(t, g))
}
func TestObservedStackIdentityFailurePrecedesObserver(t *testing.T) {
	f, clock := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			f.mu.Lock()
			f.tags["registry.example.com/app:1.1.0"] = lifecycleOldID
			f.mu.Unlock()
		}
		return nil
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	requireStackRecovery(t, out, "restored", "")
	require.Equal(t, "update.target-changed", out.ImageLifecycle.Code)
	require.Equal(t, 60, clock.waits, "only the recovery observation runs after the candidate's identity refusal")
}
func TestObservedStackFilesRestoredWithOptionalEnvAbsence(t *testing.T) {
	f, _ := observedStackFixture(t)
	require.NoError(t, os.Remove(filepath.Join(f.project.Path, ".env")))
	f.establishBaseline()
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			return errors.New("synthetic deploy failure")
		}
		return nil
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	requireStackRecovery(t, out, "restored", "")
	require.NoFileExists(t, filepath.Join(f.project.Path, ".env"))
}
