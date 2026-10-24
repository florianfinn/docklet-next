// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package imageupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestPR1ReadProjectionUsesRealPlatformProducer(t *testing.T) {
	for _, scenario := range []struct{ version, want string }{{"1.0.0", "rebuild"}, {"1.0.1", "version"}, {"index-only", "current"}, {"tag-policy", "version"}, {"identity-mismatch", "unknown"}, {"variant-mismatch", "error"}, {"tag-pattern", "version"}} {
		t.Run(scenario.want, func(t *testing.T) {
			server := httptest.NewServer(registryServer.New())
			defer server.Close()
			host := strings.TrimPrefix(server.URL, "http://")
			ref := host + "/app:stable"
			if scenario.version == "tag-policy" || scenario.version == "tag-pattern" {
				ref = host + "/app:1.0.0"
				if scenario.version == "tag-pattern" {
					ref = host + "/app:release-1.0.0"
				}
			}
			tag, err := name.NewTag(ref, name.Insecure)
			require.NoError(t, err)
			original, err := random.Image(256, 1)
			require.NoError(t, err)
			cfg, err := original.ConfigFile()
			require.NoError(t, err)
			cfg.OS = "linux"
			cfg.Architecture = "amd64"
			if scenario.version == "variant-mismatch" {
				cfg.Architecture = "arm64"
				cfg.Variant = "v8"
			}
			cfg.Config.Labels = map[string]string{"org.opencontainers.image.version": "1.0.0"}
			original, err = mutate.ConfigFile(original, cfg)
			require.NoError(t, err)
			require.NoError(t, remote.Write(tag, original, remote.WithContext(t.Context())))
			oldDigest, err := original.Digest()
			require.NoError(t, err)
			oldID, err := original.ConfigName()
			require.NoError(t, err)
			updatedConfig := *cfg
			updatedConfig.Config.Labels = map[string]string{"org.opencontainers.image.version": scenario.version, "synthetic-generation": "next"}
			updated, err := mutate.ConfigFile(original, &updatedConfig)
			require.NoError(t, err)
			if scenario.version == "tag-policy" || scenario.version == "tag-pattern" {
				candidateTag, e := name.NewTag(func() string {
					if scenario.version == "tag-pattern" {
						return host + "/app:release-1.0.1"
					}
					return host + "/app:1.0.1"
				}(), name.Insecure)
				require.NoError(t, e)
				require.NoError(t, remote.Write(candidateTag, updated, remote.WithContext(t.Context())))
			} else {
				require.NoError(t, remote.Write(tag, updated, remote.WithContext(t.Context())))
			}
			originalRoot := oldDigest.String()
			if scenario.want == "current" {
				armCfg := *cfg
				armCfg.Architecture = "arm64"
				arm, err := mutate.ConfigFile(original, &armCfg)
				require.NoError(t, err)
				before := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: original, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}}, mutate.IndexAddendum{Add: arm, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}})
				require.NoError(t, remote.WriteIndex(tag, before, remote.WithContext(t.Context())))
				beforeDigest, err := before.Digest()
				require.NoError(t, err)
				originalRoot = beforeDigest.String()
				armCfg.Config.Labels = map[string]string{"synthetic-generation": "next"}
				arm, err = mutate.ConfigFile(arm, &armCfg)
				require.NoError(t, err)
				after := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: original, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}}, mutate.IndexAddendum{Add: arm, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}})
				require.NoError(t, remote.WriteIndex(tag, after, remote.WithContext(t.Context())))
			}

			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				if strings.HasSuffix(r.URL.Path, "/containers/json") {
					if scenario.version == "tag-policy" || scenario.version == "tag-pattern" {
						_, _ = fmt.Fprintf(w, `[{"Id":"consumer","Image":%q,"ImageID":%q,"Labels":{"com.getarcaneapp.arcane.updater.strategy":"tag","com.getarcaneapp.arcane.updater.constraint":"~1.0","com.getarcaneapp.arcane.updater.tag-pattern":%q}}]`, ref, oldID.String(), func() string {
							if scenario.version == "tag-pattern" {
								return "release-(?P<version>.*)"
							}
							return ""
						}())
					} else {
						_, _ = w.Write([]byte(`[]`))
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/containers/consumer/json") {
					_, _ = fmt.Fprintf(w, `{"Id":"consumer","Image":%q,"Config":{"Image":%q,"Labels":{"com.getarcaneapp.arcane.updater.strategy":"tag","com.getarcaneapp.arcane.updater.constraint":"~1.0","com.getarcaneapp.arcane.updater.tag-pattern":%q}}}`, oldID.String(), ref, func() string {
						if scenario.version == "tag-pattern" {
							return "release-(?P<version>.*)"
						}
						return ""
					}())
					return
				}
				_, _ = fmt.Fprintf(w, `{"Id":%q,"RepoDigests":[%q],"Os":"linux","Architecture":%q,"Config":{"Labels":{"org.opencontainers.image.version":"1.0.0"}}}`, func() string {
					if scenario.version == "identity-mismatch" {
						return "sha256:" + strings.Repeat("a", 64)
					}
					return oldID.String()
				}(), host+"/app@"+originalRoot, cfg.Architecture)
			}))
			defer daemon.Close()
			db := setupImageUpdateRegistryTestDBInternal(t)
			require.NoError(t, db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			registry := registryService.NewContainerRegistryService(db, nil, nil, nil)
			svc := NewImageUpdateService(db, nil, registry, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, daemon)}, nil, nil, nil)
			result, err := svc.ReadUpdateInfo(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, scenario.want, result.State)
			if scenario.version == "identity-mismatch" || scenario.version == "variant-mismatch" {
				require.False(t, result.HasUpdate)
				require.Equal(t, "update.check-unavailable", result.Code)
				return
			}
			require.Equal(t, scenario.want != "current", result.HasUpdate)
			require.Equal(t, "linux/amd64", result.Platform)
			require.Equal(t, oldDigest.String(), result.CurrentDigest)
			require.NotEqual(t, oldID.String(), result.CurrentDigest, "config IDs must never be compared with registry manifest digests")
			var record ImageUpdateRecord
			require.NoError(t, db.First(&record, "id = ?", oldID.String()).Error)
			if scenario.version == "tag-policy" || scenario.version == "tag-pattern" {
				require.Equal(t, "current", result.ImageUpdate.State)
				require.Equal(t, "version", result.ContainerUpdates["consumer"].State)
				require.Equal(t, "current", record.UpdateInfo().State)
			} else {
				require.Equal(t, scenario.want, record.UpdateInfo().State)
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "authUsername")
			// Registry index changes confined to another platform do not alter this result.
			same, err := registry.PlatformImage(t.Context(), host+"/app@"+oldDigest.String(), v1.Platform{OS: "linux", Architecture: "amd64"})
			require.NoError(t, err)
			require.Equal(t, oldDigest.String(), same.Digest)
		})
	}
}

func TestPR1ReadProjectionDeniedAndRateLimitedAreErrors(t *testing.T) {
	for _, status := range []int{401, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			sentinel := uuid.New().String()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); _, _ = fmt.Fprint(w, sentinel) }))
			defer server.Close()
			host := strings.TrimPrefix(server.URL, "http://")
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"Id":%q,"RepoDigests":[%q],"Os":"linux","Architecture":"amd64","Config":{"Labels":{"org.opencontainers.image.version":"1.0.0"}}}`, "sha256:"+strings.Repeat("a", 64), host+"/app@sha256:"+strings.Repeat("b", 64))
			}))
			defer daemon.Close()
			db := setupImageUpdateRegistryTestDBInternal(t)
			require.NoError(t, db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			svc := NewImageUpdateService(db, nil, registryService.NewContainerRegistryService(db, nil, nil, nil), &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, daemon)}, nil, nil, nil)
			prior := ImageUpdateRecord{ID: "sha256:" + strings.Repeat("a", 64), ImageID: "sha256:" + strings.Repeat("a", 64), Repository: host + "/app", Tag: "stable", CheckTime: time.Now().Add(-time.Hour), CurrentVersion: "1.0.0", CurrentDigest: new("sha256:" + strings.Repeat("b", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64)), Platform: new("linux/amd64")}
			require.NoError(t, db.Create(&prior).Error)
			require.Equal(t, "current", prior.UpdateInfo().State)
			result, err := svc.ReadUpdateInfo(t.Context(), host+"/app:stable")
			require.NoError(t, err)
			require.Equal(t, "error", result.State)
			require.False(t, result.HasUpdate)
			if status == 401 {
				require.Equal(t, "registry.authentication-failed", result.Code)
				require.Equal(t, "authentication-required", result.AuthState)
			} else {
				require.Equal(t, "registry.rate-limited", result.Code)
			}
			var saved ImageUpdateRecord
			require.NoError(t, db.First(&saved, "id = ?", prior.ID).Error)
			require.Equal(t, "error", saved.UpdateInfo().State, "failed fresh observation must replace cached success")
			require.True(t, saved.CheckTime.After(prior.CheckTime))
			if status == 401 {
				require.Equal(t, "authentication-required", saved.UpdateInfo().AuthState)
			}
			body, e := json.Marshal(result)
			require.NoError(t, e)
			require.NotContains(t, string(body), sentinel)
		})
	}
}

func TestPR1MissingLocalImageInvalidatesCachedSuccess(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	prior := ImageUpdateRecord{ID: "image", ImageID: "image", Repository: "registry.example.com/app", Tag: "stable", CheckTime: time.Now().Add(-time.Hour), CurrentDigest: new("digest"), LatestDigest: new("digest"), Platform: new("linux/amd64")}
	require.NoError(t, db.Create(&prior).Error)
	require.Equal(t, "current", prior.UpdateInfo().State)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not found", 404) }))
	defer daemon.Close()
	svc := NewImageUpdateService(db, nil, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, daemon)}, nil, nil, nil)
	result, err := svc.ReadUpdateInfo(t.Context(), "registry.example.com/app:stable")
	require.NoError(t, err)
	require.Equal(t, "unknown", result.State)
	var saved ImageUpdateRecord
	require.NoError(t, db.First(&saved, "id = ?", prior.ID).Error)
	require.Equal(t, "unknown", saved.UpdateInfo().State)
	require.False(t, saved.UpdateInfo().HasUpdate)
}
