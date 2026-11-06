// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

type rewriteRegistryTransport struct {
	next   http.RoundTripper
	target *url.URL
}

func (r rewriteRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme, copy.URL.Host = r.target.Scheme, r.target.Host
	return r.next.RoundTrip(copy)
}

func TestRegistryImportECRBudgetAndNoPersistence(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "slow-token"}[slow], func(t *testing.T) {
			crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.NewString()})
			inner := registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0)))
			var tokenCalls, calls atomic.Int32
			var slowToken atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("X-Amz-Target") != "" {
					tokenCalls.Add(1)
					if slowToken.Load() {
						timer := time.NewTimer(150 * time.Millisecond)
						defer timer.Stop()
						select {
						case <-r.Context().Done():
							return
						case <-timer.C:
						}
					}
					w.Header().Set("Content-Type", "application/x-amz-json-1.1")
					body, err := json.Marshal(map[string]any{"authorizationData": []map[string]string{{"authorizationToken": base64.StdEncoding.EncodeToString([]byte("AWS:example"))}}})
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					_, _ = w.Write(body)
					return
				}
				user, password, ok := r.BasicAuth()
				if !ok || user != "AWS" || password != "example" {
					w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
					w.WriteHeader(401)
					return
				}
				inner.ServeHTTP(w, r)
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := rewriteRegistryTransport{next: http.DefaultTransport, target: target}
			repo := strings.TrimPrefix(server.URL, "http://") + "/app"
			tag, err := name.NewTag(repo+":1", name.Insecure)
			require.NoError(t, err)
			img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: runtime.GOOS, Architecture: runtime.GOARCH})
			require.NoError(t, err)
			require.NoError(t, remote.Write(tag, img, remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: "AWS", Password: "example"}))))
			gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, gdb.AutoMigrate(&registry.ContainerRegistry{}))
			encrypted, err := crypto.Encrypt("example")
			require.NoError(t, err)
			record := &registry.ContainerRegistry{URL: server.URL, RegistryType: registry.RegistryTypeECR, Enabled: true, Insecure: true, AWSRegion: "us-east-1", AWSAccessKeyID: "example", AWSSecretAccessKey: encrypted}
			require.NoError(t, gdb.Create(record).Error)
			svc := registry.NewContainerRegistryService(&database.DB{DB: gdb}, nil, nil, nil, &http.Client{Transport: transport})
			resolver, err := NewResolver(svc, "")
			require.NoError(t, err)
			resolver.total = time.Second
			if slow {
				resolver.perRequest = 30 * time.Millisecond
			}
			calls.Store(0)
			slowToken.Store(slow)
			result, err := resolver.Resolve(t.Context(), dto.Request{Kind: "registry", Ref: tag.Name()})
			if slow {
				requireProblem(t, err, "import.timeout", 504)
			} else {
				require.NoError(t, err)
				require.Equal(t, "registry", result.Source.Kind)
			}
			require.Equal(t, int32(1), tokenCalls.Load())
			require.LessOrEqual(t, calls.Load(), int32(6))
			var saved registry.ContainerRegistry
			require.NoError(t, gdb.First(&saved, "id = ?", record.ID).Error)
			require.Empty(t, saved.ECRToken)
			require.Nil(t, saved.ECRTokenGeneratedAt)
		})
	}
}
