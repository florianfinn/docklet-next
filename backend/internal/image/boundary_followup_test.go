// Source: docklet-next/docs/design/images-updates.md
// Revision: 48915fde5d95ed00c757116410f76617e23e8a50
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Activity-first pull and error/status regression coverage for issue #229.

package image

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	uploadDTO "github.com/florianfinn/docklet-next/types/upload"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

func TestPullActivityCreationFailureStopsBeforeDaemon(t *testing.T) {
	var calls atomic.Int32
	s, api, router, _ := imageWireFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"status":"Downloading"}`)
	})
	require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("test:fail-pull-activity", func(tx *gorm.DB) {
		if tx.Statement.Table == "activities" {
			tx.AddError(errors.New("synthetic-private activity store failure"))
		}
	}))
	rec := imageWireRequest(router, "POST", "/pull", `{"imageName":"registry.example.com/app:1"}`)
	require.Zero(t, calls.Load(), "an untracked pull must never reach the daemon")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, rec.Flushed, "the terminal frame must be flushed")
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	require.Len(t, lines, 1, rec.Body.String())
	var frame map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &frame))
	require.Equal(t, "image.pull-failed", frame["error"].(map[string]any)["code"])
	require.NotContains(t, frame, "done")
	require.NotContains(t, frame, "activityId")
	result := &huma.ValidateResult{}
	schema := api.OpenAPI().Paths["/environments/{id}/images/pull"].Post.Responses["200"].Content["application/x-json-stream"].Schema
	huma.Validate(api.OpenAPI().Components.Schemas, schema, huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, frame, result)
	require.Empty(t, result.Errors)
	require.NotContains(t, rec.Body.String(), "synthetic-private")
	// A failed admission must not poison the next healthy pull's queue slot.
	require.NoError(t, s.db.Callback().Create().Remove("test:fail-pull-activity"))
	healthy := imageWireRequest(router, "POST", "/pull", `{"imageName":"registry.example.com/app:1"}`)
	require.EqualValues(t, 1, calls.Load())
	healthyLines := strings.Split(strings.TrimSpace(healthy.Body.String()), "\n")
	require.Len(t, healthyLines, 3, healthy.Body.String())
	var started imageDTO.PullStartedFrame
	require.NoError(t, json.Unmarshal([]byte(healthyLines[0]), &started))
	require.NotEmpty(t, started.ActivityID)
	require.JSONEq(t, `{"done":true}`, healthyLines[2])
}

func TestImageUploadSessionErrorsMatchInvalidArchiveStatus(t *testing.T) {
	for _, kind := range []string{"missing", "incomplete", "wrong-kind"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			_, _, router, uploads := imageWireFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{}`)
			})
			id := "missing"
			if kind != "missing" {
				uploadKind := uploadDTO.KindImage
				filename := "image.tar"
				if kind == "wrong-kind" {
					uploadKind = uploadDTO.KindVolumeBackup
					filename = "backup.tar.gz"
				}
				session, err := uploads.CreateSession(t.Context(), uploadKind, uploadDTO.CreateSessionRequest{Filename: filename, Size: 128})
				require.NoError(t, err)
				id = session.ID
			}
			rec := imageWireRequest(router, "POST", "/upload", fmt.Sprintf(`{"uploadId":%q}`, id))
			require.Zero(t, calls.Load(), "invalid sessions must have no daemon effects")
			requireImageWireProblem(t, rec, 400, "image.invalid-archive")
		})
	}
}

func TestPullAuthenticationFailureIsTypedWithoutAnonymousRetry(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, phase := range []string{"http-401", "http-403", "stream-error", "stream-detail", "stream-denied", "stream-code", "stream-code-403"} {
			t.Run(fmt.Sprint(saved, "/", phase), func(t *testing.T) {
				var calls, authenticated atomic.Int32
				s, _, router, _ := imageWireFixture(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("X-Registry-Auth") != "" {
						authenticated.Add(1)
					}
					w.Header().Set("Content-Type", "application/json")
					switch phase {
					case "http-401", "http-403":
						status := 401
						if phase == "http-403" {
							status = 403
						}
						w.WriteHeader(status)
						_, _ = io.WriteString(w, `{"message":"synthetic-private rejected identity"}`)
					case "stream-error":
						_, _ = io.WriteString(w, `{"error":"unauthorized: synthetic-private credential"}`)
					case "stream-detail":
						_, _ = io.WriteString(w, `{"errorDetail":{"message":"access denied: synthetic-private credential"}}`)
					case "stream-denied":
						_, _ = io.WriteString(w, `{"errorDetail":{"message":"denied: requested access to the resource is denied: synthetic-private credential"}}`)
					case "stream-code", "stream-code-403":
						code := 401
						if phase == "stream-code-403" {
							code = 403
						}
						_, _ = fmt.Fprintf(w, `{"errorDetail":{"code":%d,"message":"synthetic-private credential"}}`, code)
					}
				})
				if saved {
					s.registryService = registryService.NewContainerRegistryService(s.db, nil, nil, nil)
					crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.New().String()})
					_, err := s.registryService.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "synthetic-identity", Token: uuid.New().String()})
					require.NoError(t, err)
				}
				rec := imageWireRequest(router, "POST", "/pull", `{"imageName":"registry.example.com/app:1"}`)
				require.EqualValues(t, 1, calls.Load(), "rejected authentication must not trigger a retry")
				if saved {
					require.EqualValues(t, 1, authenticated.Load())
				}
				lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
				require.Len(t, lines, 2, rec.Body.String())
				var first imageDTO.PullStartedFrame
				require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
				require.NotEmpty(t, first.ActivityID)
				var terminal imageDTO.PullErrorFrame
				require.NoError(t, json.Unmarshal([]byte(lines[1]), &terminal))
				code := "registry.authentication-required"
				if saved {
					code = "registry.authentication-failed"
				}
				require.Equal(t, code, terminal.Error.Code)
				require.NotContains(t, rec.Body.String(), "synthetic-private")
				require.NotContains(t, rec.Body.String(), `"done"`)
				var output bytes.Buffer
				err := s.PullImage(t.Context(), "registry.example.com/app:1", &output, user.SystemUser, nil)
				problem := imageDTO.Neutral(err, "unexpected", 500)
				require.Equal(t, code, problem.Problem.Code)
				require.Equal(t, 502, problem.Status, "registry identity rejection is not hub identity rejection")
				require.NotContains(t, problem.Error(), "synthetic-private")
			})
		}
	}
}

func TestImageDeadlineIsNotDaemonUnavailable(t *testing.T) {
	// The wrapped context deadline implements net.Error, but does not prove
	// that the daemon transport is unavailable.
	for _, code := range []string{"image.pull-failed", "image.tag-failed", "image.remove-failed", "image.prune-failed", "image.load-failed"} {
		t.Run(code, func(t *testing.T) {
			problem := guard.DaemonError(fmt.Errorf("synthetic-private deadline: %w", context.DeadlineExceeded), code)
			require.Equal(t, code, problem.Problem.Code)
			require.Equal(t, 502, problem.Status)
			require.NotContains(t, problem.Error(), "synthetic-private")
		})
	}
}
