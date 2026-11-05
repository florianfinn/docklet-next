// Source: docklet-next/issues/265
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original manifest-platform and concurrent shared-producer coverage.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dc "github.com/moby/moby/api/types/container"
	oci "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

var errInventoryMutation = errors.New("shared inventory changed")

func TestInventoryManifestPlatformsAndSharedPolicyReads(t *testing.T) {
	// Both containers reference one multi-platform index; the descriptor is the
	// daemon's selected manifest, not the host platform or the index platform list.
	index := oci.Index{Manifests: []oci.Descriptor{{Platform: &oci.Platform{OS: "linux", Architecture: "amd64"}}, {Platform: &oci.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}}}}
	rows := []dc.Summary{}
	for i, name := range []string{"native", "non-native", "unknown", "unraid", "uncertain", "compose"} {
		labels := map[string]string{"example.preserved": "yes"}
		if name == "unraid" {
			labels["net.unraid.docker.managed"] = ""
		}
		if name == "uncertain" {
			labels["com.docker.compose.service"] = "app"
		}
		if name == "compose" {
			labels["com.docker.compose.project"] = "stack"
			labels["com.docker.compose.service"] = "app"
		}
		row := dc.Summary{ID: "id-" + name, Names: []string{"/" + name}, Image: "registry.example.com/app:1.0.0", ImageID: "sha256:" + strings.Repeat("a", 64), Labels: labels, State: "running"}
		if i < 2 {
			descriptor := index.Manifests[i]
			row.ImageManifestDescriptor = &descriptor
		}
		rows = append(rows, row)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			require.NoError(t, json.NewEncoder(w).Encode(rows))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/info") {
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client := newTestDockerClientInternal(t, server)
	dock := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(client)
	service := containers.NewContainerService(nil, dock, nil, nil, nil)
	read := func(ctx context.Context) ([]containerDTO.Summary, error) {
		listed, e := service.ListContainersPaginated(ctx, pagination.QueryParams{Start: 0, Limit: -1}, true, false, true, "")
		return listed.Items, e
	}
	listed, e := read(t.Context())
	require.NoError(t, e)
	require.Len(t, listed, 6)
	platforms := map[string]string{}
	for _, row := range listed {
		platforms[row.ID] = row.ImagePlatform
	}
	require.Equal(t, "linux/amd64", platforms["id-native"])
	require.Equal(t, "linux/arm64/v8", platforms["id-non-native"])
	require.Equal(t, "unknown", platforms["id-unknown"])
	f := newPolicyFixture(t)
	f.p.ports.targets = func(ctx context.Context, _ string) ([]updateTarget, error) {
		rows, e := read(ctx)
		if e != nil {
			return nil, e
		}
		return projectUpdateTargets(nil, rows)
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return f.check, nil }
	require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "false"))
	var wg sync.WaitGroup
	errors := make(chan error, 24)
	for i := 0; i < 12; i++ {
		wg.Go(func() { _, e := f.s.RunUpdateCheck(t.Context()); errors <- e })
		wg.Go(func() {
			rows, e := read(t.Context())
			if e == nil {
				for _, row := range rows {
					if len(row.Names) != 1 || row.Labels["example.preserved"] != "yes" {
						e = errInventoryMutation
					}
				}
			}
			errors <- e
		})
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		require.NoError(t, e)
	}
	policies, e := f.s.ListImagePolicies(t.Context(), "0")
	require.NoError(t, e)
	require.Len(t, policies, 3)
	names := []string{}
	for _, row := range policies {
		names = append(names, row.TargetID)
	}
	require.ElementsMatch(t, []string{"native", "non-native", "unknown"}, names)
	for _, target := range []string{"unraid", "uncertain", "compose"} {
		_, e = f.s.SaveImagePolicy(t.Context(), "0", target, dto.UpdatePolicyInput{Mode: "check"}, "example-actor", "", "container")
		require.Equal(t, "container.origin-locked", policyCode(e, ""))
	}
}

func TestPolicyTargetsExcludeProtectedSelf(t *testing.T) {
	f := newPolicyFixture(t)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return projectUpdateTargets(nil, []containerDTO.Summary{{ID: "self-id", Names: []string{"/self"}, OriginFields: containerDTO.OriginFields{Origin: containerDTO.OriginStandalone}, RedeployDisabled: true}})
	}
	listed, err := f.s.ListImagePolicies(t.Context(), "0")
	require.NoError(t, err)
	require.Empty(t, listed)
	_, err = f.s.SaveImagePolicy(t.Context(), "0", "self", dto.UpdatePolicyInput{Mode: "check"}, "example-actor", "", "container")
	require.Equal(t, "container.origin-locked", policyCode(err, ""))
}
