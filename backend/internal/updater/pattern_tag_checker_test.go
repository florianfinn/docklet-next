// Source: docklet-next/issues/409
// Revision: fddfe5d38eacf1a387b773ce10d41384f8c01459
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original real-checker tag-policy records asserted against the shared web fixture.

package updater

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	registries "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
)

// PatternTagCase is one tag-policy check shared by the checker, stack preflight
// and the web adapter tests.
type patternTagCase struct {
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`
	CurrentTag string `json:"currentTag"`
	NextTag    string `json:"nextTag"`
	Update     struct {
		State          string `json:"state"`
		HasUpdate      bool   `json:"hasUpdate"`
		UpdateType     string `json:"updateType"`
		CurrentVersion string `json:"currentVersion"`
		LatestVersion  string `json:"latestVersion"`
	} `json:"update"`
}

func patternTagCases(t *testing.T) []patternTagCase {
	t.Helper()
	raw, err := os.ReadFile("../../../web/src/data/http/testdata/pattern-tag-updates.json")
	require.NoError(t, err)
	var cases []patternTagCase
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)
	return cases
}

type patternTagLister []string

func (l patternTagLister) ListTags(context.Context, string) ([]string, error) { return l, nil }

// The real configured-service checker, native observation and record projection
// produce exactly the fixture the web adapter and stack preflight consume.
func TestPatternTagCheckerMatchesTheSharedFixture(t *testing.T) {
	for _, tc := range patternTagCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			f := newPolicyFixture(t)
			registry := httptest.NewServer(registryserver.New(registryserver.Logger(log.New(io.Discard, "", 0))))
			t.Cleanup(registry.Close)
			repository := strings.TrimPrefix(registry.URL, "http://") + "/example/app"
			image := func(marker string) v1.Image {
				img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"example.marker": marker}}})
				require.NoError(t, err)
				return img
			}
			old, next := image("old"), image("next")
			oldDigest, err := old.Digest()
			require.NoError(t, err)
			nextDigest, err := next.Digest()
			require.NoError(t, err)
			id, err := old.ConfigName()
			require.NoError(t, err)
			for tag, img := range map[string]v1.Image{tc.CurrentTag: old, tc.NextTag: next} {
				ref, err := name.NewTag(repository+":"+tag, name.Insecure)
				require.NoError(t, err)
				require.NoError(t, remote.Write(ref, img, remote.WithContext(t.Context())))
			}
			effects := 0
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method != http.MethodGet:
					effects++
					w.WriteHeader(http.StatusInternalServerError)
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
				case strings.Contains(r.URL.Path, "/distribution/"):
					_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": map[string]any{"digest": nextDigest.String()}})
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = w.Write([]byte("[]"))
				case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id.String(), "Os": "linux", "Architecture": "amd64", "RepoDigests": []string{repository + "@" + oldDigest.String()}})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(daemon.Close)
			cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.49"), client.WithHTTPClient(daemon.Client()))
			require.NoError(t, err)
			t.Cleanup(func() { _ = cli.Close() })
			registrySvc := registries.NewContainerRegistryService(f.db, func(context.Context) (registries.RegistryDaemonClient, error) { return cli, nil }, kv.NewKVService(f.db), f.s.deps.Settings, registry.Client())
			observed := imageupdate.NewImageUpdateService(f.db, f.s.deps.Settings, registrySvc, &docker.DockerClientService{Client: cli}, nil, nil, nil)
			engine, err := updater.New(updater.Config{DockerClientProvider: fakeDockerClientProviderInternal{client: cli}, RegistryDigestResolver: registrySvc, RegistryTagLister: patternTagLister{tc.CurrentTag, tc.NextTag, "unrelated-9.9.9"}})
			require.NoError(t, err)
			service := &UpdaterService{engine: engine, deps: updaterDependenciesInternal{ImageUpdates: observed, RegistryDigestResolver: registrySvc}}
			serviceLabels := map[string]string{labels.LabelUpdateStrategy: "tag"}
			if tc.Pattern != "" {
				serviceLabels[labels.LabelUpdateTagPattern] = tc.Pattern
			}
			record := service.checkProjectServiceInternal(t.Context(), "example-stack", compose.ServiceConfig{Name: "app", Platform: "linux/amd64", Image: repository + ":" + tc.CurrentTag, Labels: serviceLabels}, "example-stack")
			if record.LastError != nil {
				t.Fatalf("configured observation failed: %s", *record.LastError)
			}
			info := record.UpdateInfo()
			require.Equal(t, tc.Update.State, info.State)
			require.Equal(t, tc.Update.HasUpdate, info.HasUpdate)
			require.Equal(t, tc.Update.UpdateType, info.UpdateType)
			require.Equal(t, tc.Update.CurrentVersion, info.CurrentVersion)
			require.Equal(t, tc.Update.LatestVersion, info.LatestVersion)
			require.Zero(t, effects)
		})
	}
}
