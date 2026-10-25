// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package image

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	uploadDTO "github.com/florianfinn/docklet-next/types/upload"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func imageWireFixture(t *testing.T, daemon http.HandlerFunc) (*ImageService, huma.API, *echo.Echo, *upload.UploadService) {
	t.Helper()
	s, db, activities := safetyFixture(t, daemon)
	config, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	uploads := upload.NewUploadService(config)
	router := echo.New()
	actor := user.Actor{ID: "operator", Username: "operator"}
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &actor)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("images", "test"))
	RegisterImages(api, s.dockerService, s, nil, config, activities, uploads, handlerutil.NewActivityAppContext(t.Context()))
	return s, api, router, uploads
}

func imageWireRequest(router *echo.Echo, method, suffix, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/environments/0/images"+suffix, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func requireImageWireProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	var problem imageDTO.OperationError
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	require.False(t, problem.Success)
	require.Equal(t, code, problem.Problem.Code, rec.Body.String())
	require.Equal(t, "The operation could not be completed.", problem.Problem.Message)
	require.NotContains(t, rec.Body.String(), "synthetic-private")
}

func TestRemoveHandlerPreservesTypedFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, code string
		status     int
	}{
		{"in-use", "image.in-use", 409}, {"system", "image.system-protected", 409},
		{"restore-point", "image.restore-point-protected", 409}, {"active-update", "image.active-update-protected", 409},
		{"missing", "image.not-found", 404}, {"unavailable", "image.usage-unavailable", 503},
		{"changed", "image.changed", 409}, {"failure", "image.remove-failed", 502}, {"delete-unavailable", "image.usage-unavailable", 503}, {"delete-missing", "image.not-found", 404}, {"busy", "image.busy", 409},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			id := "sha256:" + strings.Repeat("a", 64)
			var inspections, writes atomic.Int32
			s, _, router, _ := imageWireFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				p := imageDockerTestPathInternal(r.URL.Path)
				switch {
				case r.Method == "DELETE":
					writes.Add(1)
					status := 500
					if scenario.name == "delete-unavailable" {
						status = 503
					}
					if scenario.name == "delete-missing" {
						status = 404
					}
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"message":"synthetic-private delete failure"}`)
				case p == "/containers/json":
					if scenario.name == "in-use" {
						_, _ = fmt.Fprintf(w, `[{"Id":"stopped-consumer","ImageID":%q,"State":"exited"}]`, id)
					} else {
						_, _ = io.WriteString(w, `[]`)
					}
				default:
					if scenario.name == "missing" || scenario.name == "unavailable" {
						if scenario.name == "missing" {
							w.WriteHeader(404)
						} else {
							w.WriteHeader(503)
						}
						_, _ = io.WriteString(w, `{"message":"synthetic-private inspect failure"}`)
					} else if inspections.Add(1) > 1 && scenario.name == "changed" {
						_, _ = fmt.Fprintf(w, `{"Id":%q}`, "sha256:"+strings.Repeat("b", 64))
					} else {
						_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
					}
				}
			})
			if scenario.name == "system" || scenario.name == "restore-point" || scenario.name == "active-update" {
				require.NoError(t, s.db.Create(&guard.Protection{ImageID: id, Reason: scenario.name, OwnerID: "owner"}).Error)
			}
			if scenario.name == "busy" {
				release, err := s.safety.Acquire(t.Context())
				require.NoError(t, err)
				defer release()
			}
			rec := imageWireRequest(router, "DELETE", "/example?force=true", "")
			requireImageWireProblem(t, rec, scenario.status, scenario.code)
			if scenario.name != "failure" && scenario.name != "delete-unavailable" && scenario.name != "delete-missing" {
				require.Zero(t, writes.Load())
			}
		})
	}
}

func TestImageWriteHandlerWireFailures(t *testing.T) {
	for _, test := range []struct {
		name, method, suffix, body, code string
		status                           int
	}{
		{"tag-failure", "POST", "/failure/tag", `{"repository":"registry.example.com/app","tag":"1"}`, "image.tag-failed", 502},
		{"tag-unavailable", "POST", "/unavailable/tag", `{"repository":"registry.example.com/app","tag":"1"}`, "image.usage-unavailable", 503},
		{"tag-missing", "POST", "/missing/tag", `{"repository":"registry.example.com/app","tag":"1"}`, "image.not-found", 404},
		{"tag-invalid", "POST", "/source/tag", `{"repository":" "}`, "image.invalid-reference", 400},
		{"tag-reserved", "POST", "/source/tag", `{"repository":"docklet-retention.invalid/points/p","tag":"saved"}`, "image.reserved-reference", 400},
		{"pull-inline", "POST", "/pull", `{"imageName":"registry.example.com/app:1","auth":null}`, "registry.inline-credentials-not-allowed", 400},
		{"pull-invalid", "POST", "/pull", `{"imageName":"NOT valid"}`, "image.invalid-reference", 400},
		{"pull-reserved", "POST", "/pull", `{"imageName":"docklet-retention.invalid/points/p:saved"}`, "image.reserved-reference", 400},
		{"prune-invalid", "POST", "/prune", `{"mode":"invalid"}`, "image.invalid-prune", 400},
		{"prune-unavailable", "POST", "/prune", `{"mode":"all"}`, "image.usage-unavailable", 503},
		{"upload-invalid-session", "POST", "/upload", `{"uploadId":"invalid"}`, "image.invalid-archive", 404},
		{"upload-session", "POST", "/upload", `{"uploadId":"0000000000000000"}`, "image.invalid-archive", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, router, _ := imageWireFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				status := 404
				if test.name == "tag-failure" {
					status = 500
				}
				if test.name == "tag-unavailable" {
					status = 503
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"synthetic-private missing"}`)
			})
			requireImageWireProblem(t, imageWireRequest(router, test.method, test.suffix, test.body), test.status, test.code)
		})
	}
}

func imageArchiveForWireTest(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	body := `[{"Config":"config.json","RepoTags":["registry.example.com/app:1"],"Layers":[]}]`
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(body))}))
	_, err := io.WriteString(tw, body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return archive.Bytes()
}

func TestUploadHandlerReturnsTypedDaemonFailure(t *testing.T) {
	_, _, router, uploads := imageWireFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"message":"synthetic-private load failure"}`)
	})
	archive := imageArchiveForWireTest(t)
	session, err := uploads.CreateSession(t.Context(), uploadDTO.KindImage, uploadDTO.CreateSessionRequest{Filename: "image.tar", Size: int64(len(archive))})
	require.NoError(t, err)
	_, err = uploads.WriteChunk(t.Context(), uploadDTO.KindImage, session.ID, 0, archive)
	require.NoError(t, err)
	requireImageWireProblem(t, imageWireRequest(router, "POST", "/upload", fmt.Sprintf(`{"uploadId":%q}`, session.ID)), 502, "image.load-failed")
}
