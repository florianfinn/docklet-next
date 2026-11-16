// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original error propagation and durable engine-state proofs.

package caddy

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func adminURLChecked(t *testing.T, address string) (value string, err error) {
	t.Helper()
	require.NotPanics(t, func() { value, _, err = adminURL(address) })
	return
}
func TestAdminReadbackGuardsAtRealExecSeam(t *testing.T) {
	x := fixture(t)
	cases := []hostio.ExecResult{httpResult(404, "{}"), httpResult(200, "bad"), httpResult(200, "null"), httpResult(200, "{")}
	nonzero := httpResult(200, "{}")
	nonzero.ExitCode = 1
	cases = append(cases, nonzero)
	followed := httpResult(200, "{}")
	followed.Stderr = []byte("  HTTP/1.1 200 OK\n\n  HTTP/1.1 200 OK\n\n")
	cases = append(cases, followed)
	for _, res := range cases {
		x.execFn = func([]string) hostio.ExecResult { return res }
		_, err := x.e.admin(t.Context(), x.i, "localhost:2019")
		require.ErrorContains(t, err, "admin-unavailable")
	}
	x.execFn = func([]string) hostio.ExecResult { return httpResult(200, "{}") }
	_, err := x.e.admin(t.Context(), x.i, "example.com:2019")
	require.ErrorContains(t, err, "admin-unavailable")
	_, err = x.e.diverged(t.Context(), x.i, dto.Inventory{RootFile: "/etc/caddy/Caddyfile", AdminAddress: "example.com:2019"})
	require.ErrorContains(t, err, "admin-unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	x.execFn = func([]string) hostio.ExecResult { cancel(); return httpResult(200, "{}") }
	_, err = x.e.admin(ctx, x.i, "localhost:2019")
	require.ErrorContains(t, err, "exec-running")
}
func TestResumeInventoryAndPreparationErrorDiagnostics(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	for _, which := range []string{"admin", "adapt-exit", "adapt-json"} {
		x.execFn = func(args []string) hostio.ExecResult {
			if args[0] == "wget" {
				if which == "admin" {
					return hostio.ExecResult{ExitCode: 1}
				}
				return httpResult(200, "{}")
			}
			if which == "adapt-exit" {
				return hostio.ExecResult{ExitCode: 1, Stderr: []byte("adapt rejected")}
			}
			return hostio.ExecResult{Stdout: []byte("bad")}
		}
		inv, err := inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.Eligibility.Writable)
		if which == "admin" {
			require.Equal(t, dto.AdminUnavailable, inv.Eligibility.Reason)
		} else {
			require.Equal(t, dto.InvalidGraph, inv.Eligibility.Reason)
			require.Equal(t, "/etc/caddy/Caddyfile", inv.Eligibility.Path)
			if which == "adapt-exit" {
				require.Contains(t, inv.Eligibility.Detail, "adapt rejected")
			} else {
				require.Equal(t, "invalid adapted JSON", inv.Eligibility.Detail)
			}
		}
		q := x.request("create-route", baseSite())
		_, err = prepareChecked(t, x.e, q)
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		switch which {
		case "admin":
			require.Equal(t, dto.AdminUnavailable, p.Code)
		case "adapt-exit":
			require.Contains(t, p.Detail, "adapt rejected")
		case "adapt-json":
			require.Equal(t, "invalid adapted JSON", p.Detail)
		}
	}
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
	q := x.request("create-route", baseSite())
	q.Acknowledgements.Restart = true
	_, err := prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "admin-unavailable")
}
func TestUnchangedResumeGraphNeedsNoReplacementAcknowledgement(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.True(t, i.Reload)
}
func TestOwnedMetadataIdentityAndDistinctRouteNames(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	text, _ := renderChecked(t, s)
	x.owned("routes/app.caddy", text)
	require.NoError(t, os.Chmod(filepath.Join(x.root, "routes/app.caddy"), 0640))
	q := x.request("edit-route", s)
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.Equal(t, uint32(0640), i.Mode)
	q.Change.Kind = "create-route"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	q.Change.Kind = "edit-route"
	q.Change.RouteID = "wrong"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	q.Change.RouteID = q.Target
	require.NoError(t, x.d.DB.Exec("DROP TABLE proxy_verified_files").Error)
	_, err = readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "no such table")
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "no such table")
}
func TestDuplicateNamesOnlyUseSuccessfullyParsedSites(t *testing.T) {
	x := fixture(t)
	other := baseSite()
	other.Name = "other-route"
	other.Domain = "other.example.net"
	text, _ := renderChecked(t, other)
	x.put("routes/other.caddy", text)
	q := x.request("create-route", baseSite())
	_, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	x.put("routes/partial.caddy", "# docklet-route: example-app\nhttp://partial.example.net {\n respond unknown\n}\n")
	_, err = prepareChecked(t, x.e, q)
	require.NoError(t, err)
}
func TestBeforeProbeReasonAndWildcardRefusalPersist(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	x.execFn = func([]string) hostio.ExecResult {
		return hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}
	}
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	s, err := decode(i)
	require.NoError(t, err)
	require.Equal(t, dto.ProbeUnavailable, s.ProbeReason)
	wildcard := baseSite()
	wildcard.Domain = "*.example.com"
	before := len(x.commands)
	_, err = x.e.probe(t.Context(), x.i, wildcard)
	require.ErrorContains(t, err, "not-observable")
	require.Len(t, x.commands, before)
	require.ErrorContains(t, x.e.Reload(t.Context(), proxy.Intent{Engine: []byte("bad")}), "invalid character")
}
func TestRestartObservationUsesActivationDeadline(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	b, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", AdminOff: true, Before: hostio.Response{Status: 404, Headers: map[string]string{"server": "Caddy"}}, NewSite: &s})
	i := proxy.Intent{Request: dto.ApplyRequest{ID: "restart-deadline", Instance: x.i, Acknowledgements: dto.Acknowledgements{Restart: true}, Change: dto.Change{Kind: "create-route"}}, Engine: b, Restart: true}
	require.NoError(t, x.e.Restart(t.Context(), i))
	x.now = x.now.Add(2 * time.Second)
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, 30, x.waits)
}
func TestInsertImportRechecksFreshRootMount(t *testing.T) {
	for _, which := range []string{"inspect-error", "single-file"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			x.put("Caddyfile", strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n"))
			count := 0
			x.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
					count++
				}
				return false
			}
			_, err := loadChecked(t, x.e, x.i)
			require.NoError(t, err)
			boundary := count + 1
			count = 0
			x.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
					count++
					if count == boundary {
						if which == "inspect-error" {
							http.Error(w, `{"message":"inspect unavailable"}`, 500)
							return true
						}
						x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "Caddyfile"), Destination: "/etc/caddy/Caddyfile", RW: true})
					}
				}
				return false
			}
			q := dto.ApplyRequest{Instance: x.i, Target: "Caddyfile", Change: dto.Change{Kind: "insert-import"}}
			_, err = prepareChecked(t, x.e, q)
			require.Error(t, err)
		})
	}
}
