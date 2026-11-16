// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original staging faults at the real filesystem and Docker seams.

package caddy

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func validateChecked(t *testing.T, e *engine, i proxy.Intent) (err error) {
	t.Helper()
	require.NotPanics(t, func() { err = e.Validate(t.Context(), i) })
	return
}
func TestStagingRechecksInvalidGraphAndOpaquePersistence(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.StagingPath = "routes/.docklet-staging-example"
	x.put("Caddyfile", "example.com {\n")
	err = validateChecked(t, x.e, i)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "/etc/caddy/Caddyfile", p.Path)
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "persist_config off", 1))
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	x.put("routes/extra.secret.caddy", "opaque payload")
	err = validateChecked(t, x.e, i)
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.OpaqueInput, p.Code)
}
func TestStagingRefusesExistingDirectoryAndInvalidAbsolutePath(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.StagingPath = "routes/.docklet-staging-example"
	require.NoError(t, os.Mkdir(filepath.Join(x.root, i.StagingPath), 0755))
	require.Error(t, validateChecked(t, x.e, i))
	require.NoError(t, x.d.Cleanup(t.Context(), i))
	i.StagingPath = "/absolute/.docklet-staging-example"
	err = validateChecked(t, x.e, i)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, i.StagingPath, p.Path)
}
func TestStagingWriterInspectionFailureReturnsOriginalError(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.StagingPath = "routes/.docklet-staging-example"
	count := 0
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			count++
		}
		return false
	}
	_, err = loadChecked(t, x.e, x.i)
	require.NoError(t, err)
	boundary := count + 1
	count = 0
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			count++
			if count == boundary {
				http.Error(w, `{"message":"writer unavailable"}`, 500)
				return true
			}
		}
		return false
	}
	require.ErrorContains(t, validateChecked(t, x.e, i), "writer unavailable")
}
func TestStagingRelativeImportIncludesCandidateAndOriginalSibling(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", strings.Replace(rootText, "/etc/caddy/routes/*.caddy", "routes/*.caddy", 1))
	x.put("routes/other.caddy", "other.example.net {\n respond ok\n}\n")
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.StagingPath = "routes/.docklet-staging-example"
	captured := []string{}
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "caddy" && args[1] == "validate" {
			b, _ := os.ReadFile(filepath.Join(x.root, strings.TrimPrefix(args[3], "/etc/caddy/")))
			ds, _ := scan(string(b))
			for _, d := range ds {
				if d.words[0] == "import" {
					v, _ := os.ReadFile(filepath.Join(x.root, strings.TrimPrefix(d.words[1], "/etc/caddy/")))
					captured = append(captured, string(v))
				}
			}
		}
		return hostio.ExecResult{}
	}
	require.NoError(t, validateChecked(t, x.e, i))
	x.mu.Lock()
	seen := append([]string(nil), captured...)
	x.mu.Unlock()
	require.Contains(t, seen, string(i.New.Content))
	require.Contains(t, seen, "other.example.net {\n respond ok\n}\n")
	require.NoError(t, x.d.Cleanup(t.Context(), i))
	x.noTransient()
}
func TestMalformedEmptyImportGlobCannotDisappearFromStaging(t *testing.T) {
	x := fixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(x.root, "empty"), 0755))
	x.put("Caddyfile", rootText+"import /etc/caddy/empty/[.caddy\n")
	_, err := readChecked(t, x.e, x.i)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.InvalidGraph, p.Code)
	require.Contains(t, p.Detail, "syntax error in pattern")
}

// The hook cancels only after the real filesystem mkdir completed. Selection,
// confinement, metadata and Write cancellation still run in the real runtime.
type cancelStagingRuntime struct {
	*proxy.DockerRuntime
	cancel   context.CancelFunc
	attempts int
}
type cancelStagingWriter struct {
	hostio.FileWriter
	owner *cancelStagingRuntime
}

func (r *cancelStagingRuntime) Writer(ctx context.Context, i dto.Instance) (hostio.FileWriter, error) {
	w, err := r.DockerRuntime.Writer(ctx, i)
	return cancelStagingWriter{w, r}, err
}
func (w cancelStagingWriter) Mkdir(ctx context.Context, p string) error {
	if err := w.FileWriter.Mkdir(ctx, p); err != nil {
		return err
	}
	w.owner.cancel()
	return nil
}
func (w cancelStagingWriter) Write(ctx context.Context, p string, b []byte, mode uint32) error {
	w.owner.attempts++
	return w.FileWriter.Write(ctx, p, b, mode)
}
func TestStagingStopsAtFirstRealWriterCancellation(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.StagingPath = "routes/.docklet-staging-example"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := &cancelStagingRuntime{DockerRuntime: x.d, cancel: cancel}
	e := New(r)
	require.ErrorIs(t, e.Validate(ctx, i), context.Canceled)
	require.Equal(t, 1, r.attempts)
	require.NoError(t, x.d.Cleanup(t.Context(), i))
	x.noTransient()
}
