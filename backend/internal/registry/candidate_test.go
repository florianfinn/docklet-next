// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original scoped credentials, ECR token and redacted Retry-After accessor proofs.

package registry

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/kv"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestCandidateDockerConfigScopesCredentialsAndUsesECRToken(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	createTestPullRegistryInternal(t, db, "https://registry.example.com", "scanner-user", "scanner-canary")
	createTestPullRegistryInternal(t, db, "https://other.example.com", "other-user", "other-canary")
	require.NoError(t, db.Model(&ContainerRegistry{}).Where("url=?", "https://registry.example.com").Update("insecure", true).Error)
	s := NewContainerRegistryService(db, nil, nil, nil)
	raw, insecure, err := s.CandidateDockerConfig(t.Context(), "registry.example.com/app:stable")
	require.NoError(t, err)
	require.True(t, insecure)
	var config struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(raw, &config))
	require.Len(t, config.Auths, 1)
	decoded, err := base64.StdEncoding.DecodeString(config.Auths["registry.example.com"].Auth)
	require.NoError(t, err)
	require.Equal(t, "scanner-user:scanner-canary", string(decoded))
	require.NotContains(t, string(raw), "other")
	token, err := crypto.Encrypt("refreshed-ecr-token")
	require.NoError(t, err)
	secret, err := crypto.Encrypt("aws-long-term-secret")
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&ContainerRegistry{URL: "https://ecr.example.com", Enabled: true, RegistryType: RegistryTypeECR, AWSAccessKeyID: "example", AWSSecretAccessKey: secret, AWSRegion: "example-region", ECRToken: token, ECRTokenGeneratedAt: &now}).Error)
	raw, insecure, err = s.CandidateDockerConfig(t.Context(), "ecr.example.com/app:stable")
	require.NoError(t, err)
	require.False(t, insecure)
	config.Auths = nil
	require.NoError(t, json.Unmarshal(raw, &config))
	require.Len(t, config.Auths, 1)
	decoded, err = base64.StdEncoding.DecodeString(config.Auths["ecr.example.com"].Auth)
	require.NoError(t, err)
	require.Equal(t, "AWS:refreshed-ecr-token", string(decoded))
	require.NotContains(t, string(raw), "aws-long-term-secret")
}

func TestCandidateRegistryRedactsErrorsAndPersistsRetryAfter(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/" {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"errors":[{"code":"TOOMANYREQUESTS","message":"synthetic-auth-canary"}]}`)
	}))
	defer server.Close()
	require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: RegistryTypeGeneric}).Error)
	s := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil, server.Client())
	repo := strings.TrimPrefix(server.URL, "http://") + "/app"
	_, retry, err := s.CandidatePlatformImage(t.Context(), repo+":stable", v1.Platform{OS: "linux", Architecture: "amd64"})
	require.Error(t, err)
	require.True(t, retry.After(time.Now().Add(119*time.Second)))
	require.NotContains(t, err.Error(), "synthetic-auth-canary")
	before := requests
	_, nextRetry, err := s.CandidateImageTags(t.Context(), repo+":other")
	require.Error(t, err)
	require.Equal(t, before, requests, "Retry-After must prevent another registry request")
	require.True(t, retry.Equal(nextRetry))
	require.NotContains(t, output.String(), "synthetic-auth-canary")
}

func TestCandidateRetryAfterBlocksManifestFallbackRetries(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(200)
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(405)
			return
		}
		gets++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"errors":[{"code":"TOOMANYREQUESTS","message":"synthetic-auth-canary"}]}`)
	}))
	defer server.Close()
	require.NoError(t, db.Create(&ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: RegistryTypeGeneric}).Error)
	s := NewContainerRegistryService(db, nil, kv.NewKVService(db), nil, server.Client())
	start := time.Now()
	_, retry, err := s.CandidatePlatformImage(t.Context(), strings.TrimPrefix(server.URL, "http://")+"/app:stable", v1.Platform{OS: "linux", Architecture: "amd64"})
	require.Error(t, err)
	require.True(t, retry.After(start.Add(119*time.Second)))
	require.Equal(t, 1, gets, "Retry-After must prevent transport retries after manifest fallback")
	require.NotContains(t, err.Error(), "synthetic-auth-canary")
}
