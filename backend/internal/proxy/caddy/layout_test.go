// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original route-source layout and inventory projection proofs.

package caddy

import (
	"context"
	"encoding/json/v2"
	"fmt"

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

func readChecked(t *testing.T, e *engine, i dto.Instance) (r dto.ReadResult, err error) {
	t.Helper()
	require.NotPanics(t, func() { r, err = e.Read(t.Context(), i) })
	return
}
func prepareChecked(t *testing.T, e *engine, q dto.ApplyRequest) (i proxy.Intent, err error) {
	t.Helper()
	require.NotPanics(t, func() { i, err = e.Prepare(t.Context(), q) })
	return
}
func parseChecked(t *testing.T, text string) (s Site, err error) {
	t.Helper()
	require.NotPanics(t, func() { s, err = ParseSite(text) })
	return
}
func applyChecked(t *testing.T, a *proxy.Agent, q dto.ApplyRequest) (o dto.Operation, err error) {
	t.Helper()
	require.NotPanics(t, func() { o, err = a.Apply(t.Context(), q) })
	return
}
func TestP9P22SingleFileRootWithSelectedRouteDirectoryIsWritable(t *testing.T) {
	x := fixture(t)
	x.serving()
	x.c.Mounts = []container.MountPoint{{Type: mount.TypeBind, Source: filepath.Join(x.root, "Caddyfile"), Destination: "/etc/caddy/Caddyfile", RW: true}, {Type: mount.TypeBind, Source: filepath.Join(x.root, "routes"), Destination: "/etc/caddy/routes", RW: true}}
	x.i.Source = dto.Source{Kind: dto.ProjectSource, Canonical: filepath.Join(x.root, "routes"), MountTarget: "/etc/caddy/routes"}
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	require.True(t, inv.ImportPresent)
	require.Empty(t, inv.Eligibility.Reason)
	q := x.request("create-route", baseSite())
	q.Target = "app.caddy"
	q.Change.RouteID = q.Target
	o, err := applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, rootText, x.bytes("Caddyfile"))
	x.noTransient()
	x.put("Caddyfile", strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n"))
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.False(t, inv.ImportPresent)
	require.Equal(t, "import /etc/caddy/routes/*.caddy", inv.ImportLine)
}
func TestReadProjectionPreservesOwnedRoutesAndRootSiteBoundaries(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	text, _ := renderChecked(t, s)
	x.owned("routes/app.caddy", text)
	x.put("routes/unknown.caddy", "unknown.example.net {\n unknown_directive\n}\n")
	r, err := readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 3)
	require.Equal(t, "legacy.example.net", r.Routes[0].Name)
	require.Equal(t, "text-only", r.Routes[0].Class)
	require.Equal(t, "editable", r.Routes[1].Class)
	require.True(t, r.Routes[1].Eligibility.Writable)
	require.Equal(t, "text-only", r.Routes[2].Class)
	require.Equal(t, dto.UnsupportedFormat, r.Routes[2].Eligibility.Reason)
	require.Len(t, r.Files, 3)
	require.True(t, r.Files[1].Owned)
	require.Equal(t, proxy.Snapshot([]byte(text), true).Hash, r.Files[1].LastVerifiedHash)
	x.put("extra.txt", text)
	x.put("Caddyfile", rootText+"import /etc/caddy/extra.txt\n")
	r, err = readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 3)
	x.put("other/extra.caddy", text)
	x.put("Caddyfile", rootText+"import /etc/caddy/other/extra.caddy\n")
	r, err = readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 3)
	x.put("Caddyfile", rootText+"(unused) {\n respond ok\n}\n")
	r, err = readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 3)
}
func TestGlobalOptionShapesAndPersistenceWithoutResume(t *testing.T) {
	x := fixture(t)
	for _, options := range []string{"log {\n admin off\n persist_config off\n}\n", "admin off extra\npersist_config off extra\n"} {
		x.put("Caddyfile", "{\n"+options+"}\nimport /etc/caddy/routes/*.caddy\n")
		inv, err := inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.AdminOff)
		require.True(t, inv.PersistConfig)
	}
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "persist_config off", 1))
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Resume)
	require.False(t, inv.PersistConfig)
	require.True(t, inv.Eligibility.Writable)
}
func TestMalformedImportPatternsAreRefusedWithRealMatchError(t *testing.T) {
	for _, secret := range []bool{false, true} {
		t.Run(fmt.Sprint(secret), func(t *testing.T) {
			x := fixture(t)
			name := "example.caddy"
			if secret {
				name = "example.secret.caddy"
			}
			x.put("extra/"+name, "# fixture\n")
			x.put("Caddyfile", rootText+"import /etc/caddy/extra/[.caddy\n")
			_, err := readChecked(t, x.e, x.i)
			var pr *dto.Problem
			require.ErrorAs(t, err, &pr)
			require.Contains(t, pr.Detail, "syntax error in pattern")
		})
	}
	_, err := scan("example.com {\n")
	require.Error(t, err)
}
func TestNestedRootMountGuardsAndSourceEligibility(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = []string{"run", "--config", "/etc/caddy/config/Caddyfile"}
	x.i.RootFile = "/etc/caddy/config/Caddyfile"
	x.put("config/Caddyfile", "legacy.example.net {\n respond ok\n}\n")
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "config"), Destination: "/etc/caddy/config", RW: true})
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	for _, change := range []func(){func() { x.c.Mounts[1].RW = false }, func() { x.c.Mounts[1].RW = true; x.c.Mounts[1].Source = filepath.Join(x.root, "unrelated") }} {
		change()
		inv, err = inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.Eligibility.Writable)
	}
	x.c.Mounts = x.c.Mounts[:1]
	for _, change := range []func(*dto.Instance){func(i *dto.Instance) { i.Source.ReadOnly = true }, func(i *dto.Instance) { i.Source.Kind = dto.SingleFileSource }, func(i *dto.Instance) { i.Source.Kind = dto.UnknownSourceKind }} {
		i := x.i
		change(&i)
		require.False(t, x.e.rootWritable(t.Context(), i, x.c, x.i.RootFile))
	}
}
func TestPrepareAcknowledgementsCreationAndNoOpGuards(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	for _, target := range []string{"../invalid.caddy", "/absolute.caddy", "routes/hidden.secret.caddy", "other/other.caddy", "routes/other.txt"} {
		bad := q
		bad.Target = target
		_, err := prepareChecked(t, x.e, bad)
		require.ErrorContains(t, err, "invalid-input")
	}
	q.Change.Kind = "edit-route"
	q.Change.RouteID = q.Target
	_, err := prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	s := baseSite()
	text, _ := renderChecked(t, s)
	x.owned(q.Target, text)
	q.Change.Kind = "edit-route"
	q.Change.RouteID = "wrong"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
	q.Change.RouteID = q.Target
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "acknowledgement-required")
	x.put("Caddyfile", rootText)
	q.Change.Kind = "insert-import"
	q.Target = "Caddyfile"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	x.put("Caddyfile", strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n"))
	q.Change.Kind = "insert-import"
	q.Target = "Caddyfile"
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.Equal(t, x.bytes("Caddyfile")+"import /etc/caddy/routes/*.caddy\n", string(i.New.Content))
	require.Equal(t, uint32(0644), i.Mode)
	x.put("Caddyfile", strings.TrimSuffix(x.bytes("Caddyfile"), "\n"))
	i, err = prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(i.New.Content), "}\nimport "))
	x.put("Caddyfile", "")
	i, err = prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.Equal(t, "import /etc/caddy/routes/*.caddy\n", string(i.New.Content))
}
func TestProbeUnavailableReasonsAndStoppedContainer(t *testing.T) {
	for _, which := range []string{"missing", "option", "stopped"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			b, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", AdminOff: true, NewSite: &s})
			i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: "create-route"}}, Engine: b}
			x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
			x.execFn = func([]string) hostio.ExecResult {
				if which == "missing" {
					return hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}
				}
				return hostio.ExecResult{ExitCode: 1, Stderr: []byte("wget: unrecognized option")}
			}
			if which == "stopped" {
				x.c.State.Running = false
			}
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			want := dto.ProbeUnavailable
			if which == "option" {
				want = dto.ProbeOptionUnavailable
			}
			if which == "stopped" {
				want = dto.ContainerChanged
				require.Empty(t, x.commands)
			}
			require.Equal(t, want, o.Reason)
		})
	}
}

func renderChecked(t *testing.T, s Site) (text string, err error) {
	t.Helper()
	require.NotPanics(t, func() { text, err = s.Render() })
	return
}

func inventoryChecked(t *testing.T, e *engine, i dto.Instance) (r dto.Inventory, err error) {
	t.Helper()
	require.NotPanics(t, func() { r, err = e.Inventory(t.Context(), i) })
	return
}
func loadChecked(t *testing.T, e *engine, i dto.Instance) (g graph, err error) {
	t.Helper()
	require.NotPanics(t, func() { g, err = e.load(t.Context(), i) })
	return
}
