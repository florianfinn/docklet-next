// Source: getarcaneapp/arcane/backend/internal/registry/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite temp-file persistence fixture.

package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
)

func TestContainerRegistryService_InspectImageDigest_UsesStoredCredentialsForNonDockerHubRegistry(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "ghcr.io", "ghcr-user", "ghcr-token")
	wantDigest := digest.FromString("ghcr-stored-credentials").String()

	var calls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, "ghcr.io/getarcaneapp/agent:latest", imageRef)

				authCfg := decodeRegistryAuthInternal(t, options.EncodedRegistryAuth)
				assert.Equal(t, "ghcr-user", authCfg.Username)
				assert.Equal(t, "ghcr-token", authCfg.Password)

				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "ghcr.io/getarcaneapp/agent:latest", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.Equal(t, "ghcr-user", result.AuthUsername)
	assert.True(t, result.UsedCredential)
}

func TestContainerRegistryService_InspectImageDigest_UsesStoredCredentialsInsteadOfRateLimitedAnonymous(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "ghcr.io", "ghcr-user", "ghcr-token")
	wantDigest := digest.FromString("ghcr-rate-limit").String()

	var anonymousCalls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				if options.EncodedRegistryAuth == "" {
					anonymousCalls++
					return client.DistributionInspectResult{}, errors.New(
						"Error response from daemon: toomanyrequests: retry-after: 1.246809ms, allowed: 44000/minute",
					)
				}

				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "ghcr.io/getarcaneapp/agent:latest", nil)
	require.NoError(t, err)
	assert.Zero(t, anonymousCalls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.True(t, result.UsedCredential)
}

func TestContainerRegistryService_InspectImageDigest_FallsBackToAnonymousWhenStoredCredentialRejected(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "ghcr.io", "ghcr-user", "expired-token")
	wantDigest := digest.FromString("ghcr-anonymous-fallback").String()

	var anonymousCalls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				if options.EncodedRegistryAuth != "" {
					return client.DistributionInspectResult{}, errors.New("Error response from daemon: unauthorized: authentication required")
				}

				anonymousCalls++
				return client.DistributionInspectResult{
					Descriptor: ocispec.Descriptor{
						Digest: digest.Digest(wantDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "ghcr.io/immich-app/immich-server:release", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, anonymousCalls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.False(t, result.UsedCredential)
}

// A rate-limited credential must not fall back to anonymous: both paths share the per-registry quota.
func TestContainerRegistryService_InspectImageDigest_DoesNotFallBackToAnonymousOnCredentialRateLimit(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "ghcr.io", "ghcr-user", "ghcr-token")

	var anonymousCalls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				if options.EncodedRegistryAuth == "" {
					anonymousCalls++
				}
				return client.DistributionInspectResult{}, errors.New(
					"Error response from daemon: toomanyrequests: retry-after: 1.246809ms, allowed: 44000/minute",
				)
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "ghcr.io/getarcaneapp/agent:latest", nil)
	require.Error(t, err)
	assert.Zero(t, anonymousCalls, "must not burn anonymous quota when the credential itself is rate limited")
	require.NotNil(t, result)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.True(t, result.UsedCredential)
}

// The direct-HTTP fallback must not spend an anonymous request when a stored credential resolves the digest.
func TestContainerRegistryService_InspectImageDigest_FallbackUsesStoredCredentialsBeforeAnonymous(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "ghcr.io", "ghcr-user", "ghcr-token")
	wantDigest := digest.FromString("fallback-stored-credentials").String()

	var anonymousCalls int
	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("Error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// The version check never carries credentials; only manifest requests count.
			if req.URL.Path == "/v2/" {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}, nil
			}
			authorization := req.Header.Get("Authorization")
			if authorization == "" {
				anonymousCalls++
				return nil, errors.New("unexpected anonymous registry request")
			}
			assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("ghcr-user:ghcr-token")), authorization)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Docker-Content-Digest": []string{wantDigest},
					"Content-Type":          []string{"application/vnd.docker.distribution.manifest.v2+json"},
				},
				Body: http.NoBody,
			}, nil
		}),
	}

	result, err := svc.InspectImageDigest(t.Context(), "ghcr.io/getarcaneapp/agent:latest", nil)
	require.NoError(t, err)
	assert.Zero(t, anonymousCalls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.Equal(t, "ghcr-user", result.AuthUsername)
	assert.True(t, result.UsedCredential)
}

func TestContainerRegistryService_InspectImageDigest_FallsBackWhenDistributionNotFound(t *testing.T) {
	wantDigest := digest.FromString("fallback-not-found").String()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryPingInternal(w, r, "") {
			return
		}
		if r.URL.Path == "/v2/team/app/manifests/1.2.3" {
			writeManifestHeadInternal(w, wantDigest)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	var calls int
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, serverURL.Host+"/team/app:1.2.3", imageRef)
				assert.Empty(t, options.EncodedRegistryAuth)
				return client.DistributionInspectResult{}, errors.New("Error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = server.Client()

	result, err := svc.InspectImageDigest(t.Context(), serverURL.Host+"/team/app:1.2.3", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, serverURL.Host, result.AuthRegistry)
}

func TestContainerRegistryService_InspectImageDigest_FallsBackWhenDistributionForbidden(t *testing.T) {
	wantDigest := digest.FromString("fallback-forbidden").String()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryPingInternal(w, r, "") {
			return
		}
		if r.URL.Path == "/v2/team/app/manifests/1.2.3" {
			writeManifestHeadInternal(w, wantDigest)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	var calls int
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, serverURL.Host+"/team/app:1.2.3", imageRef)
				assert.Empty(t, options.EncodedRegistryAuth)
				return client.DistributionInspectResult{}, errors.New("Error response from daemon: <html><body><h1>403 Forbidden</h1> Request forbidden by administrative r" +
					"ules. </body></html>")
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = server.Client()

	result, err := svc.InspectImageDigest(t.Context(), serverURL.Host+"/team/app:1.2.3", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, wantDigest, result.Digest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, serverURL.Host, result.AuthRegistry)
}

func TestContainerRegistryService_InspectImageDigest_RetriesStoredCredentialsAfterRegistryAuth403(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	wantDigest := digest.FromString("stored-credential-fallback").String()

	var authHeaders []string
	var tokenURL string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		challenge := `Bearer realm="` + tokenURL + `",service="registry.example.com"`
		if registryPingInternal(w, r, challenge) {
			return
		}
		switch r.URL.Path {
		case "/v2/team/app/manifests/1.2.3":
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			switch r.Header.Get("Authorization") {
			case "Bearer credential-token":
				w.WriteHeader(http.StatusForbidden)
			case "Bearer anonymous-token":
				writeManifestHeadInternal(w, wantDigest)
			default:
				w.Header().Set("WWW-Authenticate", challenge)
				w.WriteHeader(http.StatusUnauthorized)
			}
		case "/token":
			username, password, ok := r.BasicAuth()
			if !ok {
				if !assert.Empty(t, r.Header.Get("Authorization")) {
					return
				}
				if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{
					"token": "anonymous-token",
				})) {
					return
				}
				return
			}
			if !assert.Equal(t, "stored-user", username) {
				return
			}
			if !assert.Equal(t, "stored-token", password) {
				return
			}
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"token": "credential-token",
			})) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	tokenURL = server.URL + "/token"

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	createTestPullRegistryInternal(t, db, server.URL, "stored-user", "stored-token")

	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("Error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = server.Client()

	result, err := svc.InspectImageDigest(t.Context(), serverURL.Host+"/team/app:1.2.3", nil)
	require.NoError(t, err)
	assert.Equal(t, wantDigest, result.Digest)
	// Stored credentials go first; only after rejection does the lookup retry anonymously.
	assert.Equal(t, []string{"Bearer credential-token", "Bearer anonymous-token"}, authHeaders)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.False(t, result.UsedCredential)
}

func TestContainerRegistryService_InspectImageDigest_DoesNotFallbackOnTLSFailure(t *testing.T) {
	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				assert.Equal(t, "registry.example.com/team/app:1.2.3", imageRef)
				assert.Empty(t, options.EncodedRegistryAuth)
				return client.DistributionInspectResult{}, errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")
			},
		}, nil
	}, nil, nil)

	result, err := svc.InspectImageDigest(t.Context(), "registry.example.com/team/app:1.2.3", nil)
	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, strings.ToLower(err.Error()), "x509")
	assert.NotContains(t, err.Error(), "registry fallback failed")
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, "registry.example.com", result.AuthRegistry)
}

func TestContainerRegistryService_InspectImageDigest_PreservesDaemonAndFallbackErrors(t *testing.T) {
	daemonErr := errors.New("Error response from daemon: Not Found")
	fallbackErr := errors.New("dial tcp: i/o timeout")

	svc := NewContainerRegistryService(nil, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, daemonErr
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, fallbackErr
		}),
	}

	result, err := svc.InspectImageDigest(t.Context(), "registry.example.com/team/app:1.2.3", nil)
	require.Error(t, err)
	require.NotNil(t, result)
	require.ErrorIs(t, err, daemonErr)
	assert.ErrorIs(t, err, fallbackErr)
}

func TestContainerRegistryService_InspectImageDigest_PreservesAnonymousUnauthorizedWhenCredentialLookupFails(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	var tokenURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		challenge := `Bearer realm="` + tokenURL + `",service="registry.example.com"`
		if registryPingInternal(w, r, challenge) {
			return
		}
		switch r.URL.Path {
		case "/v2/team/app/manifests/1.2.3":
			w.Header().Set("WWW-Authenticate", challenge)
			w.WriteHeader(http.StatusUnauthorized)
		case "/token":
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"token": "anonymous-token",
			})) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	tokenURL = server.URL + "/token"

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	svc := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("Error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil)
	svc.distributionHTTPClient = server.Client()

	result, err := svc.InspectImageDigest(t.Context(), serverURL.Host+"/team/app:1.2.3", nil)
	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Contains(t, err.Error(), "anonymous access unauthorized")
	assert.Contains(t, err.Error(), "401 Unauthorized")
	assert.Contains(t, err.Error(), "failed to load enabled registries")
}

func createRegistryWithRepositoryNames(t *testing.T, svc *ContainerRegistryService, repositoryNames ...string) *ContainerRegistry {
	t.Helper()

	registryRecord, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:             "https://registry.example.com",
		Username:        "my-user",
		Token:           "my-token",
		RepositoryNames: repositoryNames,
	})
	require.NoError(t, err)

	return registryRecord
}

func fetchRegistry(t *testing.T, db *database.DB, id string) ContainerRegistry {
	t.Helper()

	var fetched ContainerRegistry
	require.NoError(t, db.WithContext(t.Context()).First(&fetched, "id = ?", id).Error)

	return fetched
}

func TestContainerRegistryService_CreateRegistry_NormalizesAndPersistsRepositoryNames(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	// Entries are trimmed, empties dropped and duplicates removed while
	// preserving first-occurrence order.
	registryRecord := createRegistryWithRepositoryNames(t, svc, " team ", "", "team", "team/platform", " team ")
	assert.Equal(t, database.StringSlice{"team", "team/platform"}, registryRecord.RepositoryNames)
	assert.Equal(t, database.StringSlice{"team", "team/platform"}, fetchRegistry(t, db, registryRecord.ID).RepositoryNames)
}

func TestContainerRegistryService_CreateRegistry_RejectsInvalidRepositoryName(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	_, err := svc.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{
		URL:             "https://registry.example.com",
		RepositoryNames: []string{"team:latest"},
	})
	require.ErrorIs(t, err, common.ErrValidation)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	assert.Equal(t, "repositoryNames", fieldErr.Field)
}

func TestContainerRegistryService_UpdateRegistry_RepositoryNamesPointerSemantics(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	svc := NewContainerRegistryService(db, nil, nil, nil)

	registryRecord := createRegistryWithRepositoryNames(t, svc, "team", "team/platform")

	// A nil pointer leaves the existing names untouched.
	updated, err := svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		Username: new("updated-user"),
	})
	require.NoError(t, err)
	assert.Equal(t, database.StringSlice{"team", "team/platform"}, updated.RepositoryNames)

	// An empty slice clears them.
	updated, err = svc.UpdateRegistry(t.Context(), registryRecord.ID, containerregistry.UpdateContainerRegistryRequest{
		RepositoryNames: &[]string{},
	})
	require.NoError(t, err)
	assert.Empty(t, updated.RepositoryNames)
	assert.Empty(t, fetchRegistry(t, db, registryRecord.ID).RepositoryNames)
}

func TestListImageTagsCredentialsInternal(t *testing.T) {
	for _, tt := range []struct {
		name         string
		external     []containerregistry.Credential
		wantUser     string
		wantPassword string
	}{
		{name: "stored", wantUser: "stored-user", wantPassword: "stored-token"},
		{
			name: "external overrides stored",
			external: []containerregistry.Credential{{
				URL:      "docker.io",
				Username: "external-user",
				Token:    "external-token",
				Enabled:  true,
			}},
			wantUser:     "external-user",
			wantPassword: "external-token",
		},
		{name: "external excludes unrelated stored", external: []containerregistry.Credential{{URL: "other.test", Username: "external-user", Token: "external-token", Enabled: true}}},
		{name: "disabled external ignored", external: []containerregistry.Credential{{URL: "docker.io", Username: "external-user", Token: "external-token", Enabled: false}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			createTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "stored-user", "stored-token")
			calls := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "registry-1.docker.io", req.URL.Host)
				if req.URL.Path == "/v2/" {
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
				}
				calls++
				assert.Equal(t, "/v2/library/nginx/tags/list", req.URL.Path)
				user, password, _ := req.BasicAuth()
				assert.Equal(t, tt.wantUser, user)
				assert.Equal(t, tt.wantPassword, password)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"name":"library/nginx","tags":["1.2.3","1.2.4"]}`)), Header: http.Header{}}, nil
			})}
			svc := NewContainerRegistryService(db, nil, nil, nil, httpClient)
			tags, err := svc.ListImageTags(t.Context(), "nginx:1.2.3", tt.external)
			require.NoError(t, err)
			assert.Equal(t, []string{"1.2.3", "1.2.4"}, tags)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestListImageTagsDoesNotFallBackOnRateLimitInternal(t *testing.T) {
	// The registry client retries 429s itself, so count credentialed and anonymous listings rather than requests.
	credentialed, anonymous := 0, 0
	svc := NewContainerRegistryService(nil, nil, nil, nil, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v2/" {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
		}
		if req.Header.Get("Authorization") == "" {
			anonymous++
		} else {
			credentialed++
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: http.NoBody, Header: http.Header{}}, nil
	})})
	tags, err := svc.ListImageTags(
		t.Context(),
		"registry.test/team/app:1.0.0",
		[]containerregistry.Credential{{
			URL:      "registry.test",
			Username: "user",
			Token:    "token",
			Enabled:  true,
		}},
	)
	require.Error(t, err)
	assert.Nil(t, tags)
	assert.Positive(t, credentialed)
	assert.Zero(t, anonymous, "a rate-limited credential must not spend the anonymous quota")
}

func TestListImageTagsCancellationInternal(t *testing.T) {
	svc := NewContainerRegistryService(nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.ListImageTags(ctx, "registry.test/team/app:1.0.0", nil)
	require.ErrorIs(t, err, context.Canceled, "error = %v", err)
}

func TestListImageTagsRejectedCredentialFallbackInternal(t *testing.T) {
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:token"))
	for _, tt := range []struct {
		name          string
		host          string
		wantError     bool
		wantTokenAuth []string
	}{
		{name: "private registry public image", host: "registry.test", wantTokenAuth: []string{basic, ""}},
		{name: "docker hub preserves quota", host: "docker.io", wantError: true, wantTokenAuth: []string{basic}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The registry challenges at /v2/ and its token endpoint rejects the stored credential but serves anonymous tokens.
			var tokenAuth []string
			svc := NewContainerRegistryService(nil, nil, nil, nil, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				response := &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}
				switch req.URL.Path {
				case "/v2/":
					response.StatusCode = http.StatusUnauthorized
					response.Header.Set("WWW-Authenticate", `Bearer realm="https://auth.test/token"`)
				case "/token":
					tokenAuth = append(tokenAuth, req.Header.Get("Authorization"))
					if req.Header.Get("Authorization") != "" {
						response.StatusCode = http.StatusUnauthorized
					} else {
						response.Body = io.NopCloser(strings.NewReader(`{"token":"anonymous-token"}`))
					}
				default:
					response.Body = io.NopCloser(strings.NewReader(`{"tags":["1.0.1"]}`))
				}
				return response, nil
			})})
			tags, err := svc.ListImageTags(t.Context(), tt.host+"/team/app:1.0.0", []containerregistry.Credential{{URL: tt.host, Username: "user", Token: "token", Enabled: true}})
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, tags)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{"1.0.1"}, tags)
			}
			assert.Equal(t, tt.wantTokenAuth, tokenAuth)
		})
	}
}

// newLabelTestRegistryInternal starts an in-memory OCI registry and returns its
// host as "localhost:PORT" so go-containerregistry resolves it over plain HTTP.
func newLabelTestRegistryInternal(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return "localhost:" + serverURL.Port()
}

func pushLabeledImageInternal(t *testing.T, imageRef string, imageLabels map[string]string) v1.Image {
	t.Helper()
	img, err := random.Image(64, 1)
	require.NoError(t, err)

	if imageLabels != nil {
		cfg, configFileErr := img.ConfigFile()
		require.NoError(t, configFileErr)
		cfg = cfg.DeepCopy()
		cfg.Config.Labels = imageLabels
		img, configFileErr = mutate.ConfigFile(img, cfg)
		require.NoError(t, configFileErr)
	}

	ref, err := name.ParseReference(imageRef)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))
	return img
}

func TestContainerRegistryService_ImageVersionLabelInternal(t *testing.T) {
	host := newLabelTestRegistryInternal(t)
	imageRef := host + "/getarcaneapp/arcane:next"
	img := pushLabeledImageInternal(t, imageRef, map[string]string{
		ociImageVersionLabel: "v2.8.0-next.66",
	})

	svc := NewContainerRegistryService(nil, nil, nil, nil)

	label, err := svc.ImageVersionLabel(t.Context(), imageRef)
	require.NoError(t, err)
	assert.Equal(t, "v2.8.0-next.66", label)

	// Digest references resolve too — the version service prefers them.
	imgDigest, err := img.Digest()
	require.NoError(t, err)
	label, err = svc.ImageVersionLabel(t.Context(), host+"/getarcaneapp/arcane@"+imgDigest.String())
	require.NoError(t, err)
	assert.Equal(t, "v2.8.0-next.66", label)
}

func TestContainerRegistryService_ImageVersionLabelMissingLabelInternal(t *testing.T) {
	host := newLabelTestRegistryInternal(t)
	imageRef := host + "/getarcaneapp/arcane:next"
	pushLabeledImageInternal(t, imageRef, nil)

	svc := NewContainerRegistryService(nil, nil, nil, nil)
	_, err := svc.ImageVersionLabel(t.Context(), imageRef)
	assert.ErrorIs(t, err, ErrNoVersionLabel)
}

func TestContainerRegistryService_ImageVersionLabelErrorNotCachedInternal(t *testing.T) {
	host := newLabelTestRegistryInternal(t)
	imageRef := host + "/getarcaneapp/arcane:next"

	svc := NewContainerRegistryService(nil, nil, nil, nil)

	// The image does not exist yet: the lookup must fail...
	_, err := svc.ImageVersionLabel(t.Context(), imageRef)
	require.Error(t, err)

	// ...and the failure must not be cached: after the push the same ref resolves.
	pushLabeledImageInternal(t, imageRef, map[string]string{
		ociImageVersionLabel: "v2.8.0-next.67",
	})
	label, err := svc.ImageVersionLabel(t.Context(), imageRef)
	require.NoError(t, err)
	assert.Equal(t, "v2.8.0-next.67", label)
}
