// Source: docklet-next/issues/594
// Revision: 275fa97a5a1ca374640db683cd6e1269e9b2e995
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original eligibility and admission agreement proof for an unselected interpolated image.

package project

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

// An unchanged interpolated sidecar does not refuse an update that installs only
// its literal peer: eligibility and admission agree (closing review M38-CL-R01).
// Updating the interpolated service itself is still refused.
func TestUnselectedInterpolatedImageDoesNotRefuseItsPeer(t *testing.T) {
	sidecars := map[string]struct {
		image   func(ref string) string
		refusal imageDTO.Binding
	}{
		"interpolated": {func(ref string) string { return "${SIDE_IMAGE:-" + ref + "} # stays unchanged" }, imageDTO.Binding{Scope: "service", Service: "side", Reason: "variable-reference", Variable: "SIDE_IMAGE"}},
		"folded":       {func(ref string) string { return ">-\n      " + ref }, imageDTO.Binding{Scope: "service", Service: "side", Reason: "unparsable"}},
	}
	for kind, sidecar := range sidecars {
		for _, selection := range [][]string{{"app"}, nil, {"side"}} {
			t.Run(kind+"-"+strings.Join(append([]string{"selection"}, selection...), "-"), func(t *testing.T) {
				f := newLifecycleFixture(t)
				server := httptest.NewServer(registryServer.New())
				t.Cleanup(server.Close)
				host := strings.TrimPrefix(server.URL, "http://")
				oldRef, newRef, sideRef := host+"/app:1.0.0", host+"/app:1.1.0", host+"/side:1.0.0"
				image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
				require.NoError(t, err)
				tag, err := name.NewTag(newRef, name.Insecure)
				require.NoError(t, err)
				require.NoError(t, remote.Write(tag, image, remote.WithContext(t.Context())))
				sideTag, err := name.NewTag(sideRef, name.Insecure)
				require.NoError(t, err)
				require.NoError(t, remote.Write(sideTag, image, remote.WithContext(t.Context())))
				require.NoError(t, f.db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
				labels := "    labels:\n      com.getarcaneapp.arcane.updater.strategy: tag\n"
				source := "services:\n  app:\n    image: " + oldRef + "\n" + labels + "  side:\n    image: " + sidecar.image(sideRef) + "\n"
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
				f.tags[oldRef], f.tags[sideRef] = lifecycleOldID, lifecycleOldID
				app := f.workloads["app"]
				app.Reference = oldRef
				f.workloads["app"] = app
				side := app
				side.ID = strings.Repeat("1", 64)
				side.Service = "side"
				side.Reference = sideRef
				f.workloads["side"] = side
				eligibility, err := f.service.ImageUpdateEligibility(t.Context(), f.project.ID)
				require.NoError(t, err)
				require.Nil(t, eligibility.Project)
				for _, entry := range eligibility.Services {
					require.Equal(t, entry.Service == "app", entry.Eligible, entry.Service)
				}
				current := f.establishBaseline()
				f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
					return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{
						"app":  {UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: true, State: "version", CurrentVersion: "1.0.0", LatestVersion: "1.1.0"}},
						"side": {UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: selection != nil && selection[0] == "side", State: "version", CurrentVersion: "1.0.0", LatestVersion: "1.1.0"}},
					}}, nil
				})
				intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: selection})
				if selection != nil && selection[0] == "side" {
					var refusal *imageDTO.OperationError
					require.ErrorAs(t, err, &refusal)
					require.Equal(t, sidecar.refusal, *refusal.Problem.Binding)
					return
				}
				require.NoError(t, err)
				require.Len(t, intent.Selections, 1)
				require.Equal(t, "app", intent.Selections[0].Service)
				desired, err := lifecycleVersionFiles(current, intent.Selections)
				require.NoError(t, err)
				for _, file := range desired.Files {
					if file.Path == "compose.yaml" {
						require.Equal(t, strings.Replace(source, oldRef, newRef, 1), string(file.Content))
					}
				}
				assertNoServiceAdmission(t, f)
			})
		}
	}
}

// Eligibility and the admission writer agree per service by construction: a
// service is eligible exactly when the writer accepts a selection of it alone,
// and a document-wide writer refusal is a project binding (M38-CL-R2-01).
func TestEligibilityAgreesWithTheAdmissionWriter(t *testing.T) {
	for _, tc := range []struct{ name, prefix, suffix string }{
		{"literal-only", "", "  side:\n    image: registry.example.com/side:1.0.0\n"},
		{"unselected-shared-image-anchor", "", "  side:\n    image: &side_image registry.example.com/side:1.0.0\n  peer:\n    image: *side_image\n"},
		{"unselected-label-anchor", "", "  side:\n    image: registry.example.com/side:1.0.0\n    labels: &side_labels {example.label: example}\n"},
		{"unselected-merge-key", "x-common: &common\n  restart: unless-stopped\n", "  side:\n    <<: *common\n    image: registry.example.com/side:1.0.0\n"},
		{"unselected-interpolation", "", "  side:\n    image: ${SIDE_IMAGE:-registry.example.com/side:1.0.0}\n"},
		{"unselected-folded-scalar", "", "  side:\n    image: >-\n      registry.example.com/side:1.0.0\n"},
		{"unselected-literal-scalar", "", "  side:\n    image: |-\n      registry.example.com/side:1.0.0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(tc.prefix+lifecycleOriginal+tc.suffix), 0600))
			eligibility, err := f.service.ImageUpdateEligibility(t.Context(), f.project.ID)
			require.NoError(t, err)
			snapshot, err := f.service.lifecycleInputs(t.Context(), f.project)
			require.NoError(t, err)
			for name, svc := range snapshot.Model.Services {
				_, writerErr := lifecycleVersionFiles(snapshot, []dto.ImageSelection{{Service: name, SourceReference: svc.Image, Reference: svc.Image, Kind: "rebuild"}})
				var refusal *imageDTO.OperationError
				if writerErr != nil {
					require.ErrorAs(t, writerErr, &refusal)
				}
				if eligibility.Project != nil {
					// A document refusal refuses every service; admission may name a more specific reason first.
					require.Error(t, writerErr, name)
					continue
				}
				var entry dto.ServiceEligibility
				for _, row := range eligibility.Services {
					if row.Service == name {
						entry = row
					}
				}
				require.Equal(t, writerErr == nil, entry.Eligible, name)
				if writerErr != nil {
					require.Equal(t, *refusal.Problem.Binding, *entry.Binding, name)
				}
			}
		})
	}
}

// Eligibility of a configured 256-service stack stays within the remote request
// budget: the document is parsed once, not once per service (M38-CL-R3-02).
// Allocation is the deterministic proxy; the per-service writer dry run needed
// about 20 times this bound.
func TestEligibilityCostOfALargeStack(t *testing.T) {
	f := newLifecycleFixture(t)
	var source strings.Builder
	source.WriteString("services:\n")
	for i := range 256 {
		fmt.Fprintf(&source, "  svc%03d:\n    image: registry.example.com/app:1.0.0\n    environment:\n", i)
		for j := range 32 {
			fmt.Fprintf(&source, "      EXAMPLE_%02d: %s\n", j, strings.Repeat("x", 64))
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source.String()), 0600))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	result, err := f.service.ImageUpdateEligibility(t.Context(), f.project.ID)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.Nil(t, result.Project)
	require.Len(t, result.Services, 256)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(2<<30), "allocated bytes")
}
