// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Real daemon and registry transport outcome tests for issue #229.

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"
)

func TestRegistryProbeClassifiesActualDaemonResponses(t *testing.T) {
	for _, test := range []struct {
		status        int
		message, code string
		want          int
	}{
		{401, "synthetic rejected identity", "registry.authentication-failed", 502},
		{403, "synthetic access denied", "registry.authentication-failed", 502},
		{429, "synthetic limit", "registry.rate-limited", 429},
		{500, "synthetic connection failure", "registry.unavailable", 503},
		// Docker commonly wraps a registry's HTTP error in a daemon 500.
		{500, "unauthorized: synthetic rejected credential", "registry.authentication-failed", 502},
		{500, "toomanyrequests: synthetic limit", "registry.rate-limited", 429},
	} {
		t.Run(fmt.Sprint(test.status, test.code), func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": test.message})
			}))
			defer daemon.Close()
			docker, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer docker.Close()
			s := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) { return docker, nil }, nil, nil)
			reg, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
			require.NoError(t, err)
			_, err = NewHandler(s, nil).TestRegistry(t.Context(), &TestContainerRegistryInput{ID: reg.ID})
			require.Error(t, err)
			result, ok := err.(*containerregistry.RegistryTestResult)
			require.True(t, ok)
			require.Equal(t, test.want, result.GetStatus())
			require.Equal(t, test.code, result.Code)
			body, e := json.Marshal(result)
			require.NoError(t, e)
			require.NotContains(t, string(body), test.message)
		})
	}
}
func TestRegistryPlatformAnonymousAuthAndRateLimit(t *testing.T) {
	for _, test := range []struct {
		status int
		code   string
	}{{401, "registry.authentication-required"}, {403, "registry.authentication-required"}, {429, "registry.rate-limited"}, {503, "registry.unavailable"}} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = fmt.Fprint(w, `{"errors":[{"code":"DENIED","message":"synthetic diagnostic"}]}`)
			}))
			defer server.Close()
			s := NewContainerRegistryService(db, nil, nil, nil)
			require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, RegistryType: "generic", Insecure: true, Enabled: true}).Error)
			observation, err := s.PlatformImage(t.Context(), strings.TrimPrefix(server.URL, "http://")+"/app:1.0.0", v1.Platform{OS: "linux", Architecture: "amd64"})
			require.Error(t, err)
			require.Equal(t, test.code, imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
			if test.status == 401 || test.status == 403 {
				require.Equal(t, "authentication-required", observation.AuthState)
			}
		})
	}
}
