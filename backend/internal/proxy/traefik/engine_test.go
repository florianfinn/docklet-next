// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP1P2P3SiblingGraphAndStagedValidation(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	require.NoError(t, x.d.Stage(t.Context(), in))
	require.NoError(t, x.validate(in))
	for _, tt := range []struct {
		name, content string
		want          dto.Reason
	}{
		{"nested/BROKEN.YAML", "http: [", dto.InvalidGraph},
		{"nested/file.TOML", "[http.routers]", dto.UnsupportedFormat},
		{"wrong.yml", "http: {routers: {bad: {rule: 7, service: app}}}", dto.InvalidGraph},
		{"rule.yml", "http: {routers: {bad: {rule: 'Unknown(`x`)', service: app}}}", dto.InvalidGraph},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x.put("dynamic/"+tt.name, tt.content)
			err := x.validate(in)
			requireCode(t, err, tt.want)
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "dynamic/"+tt.name, p.Path)
			require.Equal(t, tiny, x.read(r.Target))
			require.NoError(t, os.Remove(filepath.Join(x.root, "dynamic", tt.name)))
		})
	}
	x.put(in.TempPath, strings.Replace(string(in.New.Content), "Host(`next.example.com`)", "Unknown(`x`)", 1))
	requireCode(t, x.validate(in), dto.InvalidGraph)
	x.put(in.TempPath, string(in.New.Content))
	x.put("dynamic/added.yml", "http: {middlewares: {extra: {headers: {}}}}\n")
	g, err := x.d.Graph(t.Context(), x.i)
	require.NoError(t, err)
	require.Contains(t, g, "dynamic/added.yml")
	requireCode(t, x.d.Publish(t.Context(), in), dto.StaleHash)
	require.Equal(t, tiny, x.read(r.Target))
}
func TestP14OpaqueFilesAndP22Eligibility(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/nested/AUTH.SECRET.YAML", "must-not-project")
	x.put("dynamic/nested.secret/routes.yml", "must-not-project")
	require.NoError(t, os.Symlink("/nonexistent", filepath.Join(x.root, "dynamic", "link.secret.yml")))
	read, err := x.readResult()
	require.NoError(t, err)
	require.Len(t, read.OpaqueFiles, 3)
	b, err := json.Marshal(read)
	require.NoError(t, err)
	require.NotContains(t, string(b), "opaque-payload-tag")
	require.NotContains(t, string(b), "must-not-project")
	for _, p := range x.archives {
		require.False(t, hostio.Secret(p), p)
	}
	for _, tt := range []struct {
		name, content string
		want          dto.Reason
	}{
		{"extra.toml", "[http]", dto.UnsupportedFormat},
		{"template.yml", "http: {}\n# {{ .Name }}\n", dto.TemplateInput},
		{"wrong.yml", "http: {routers: []}", dto.InvalidGraph},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x.put("dynamic/"+tt.name, tt.content)
			read, err := x.readResult()
			require.NoError(t, err)
			require.False(t, read.Eligibility.Writable)
			require.Equal(t, tt.want, read.Eligibility.Reason)
			_, err = x.e.Prepare(t.Context(), x.request("edit-route"))
			requireCode(t, err, tt.want)
			require.NoError(t, os.Remove(filepath.Join(x.root, "dynamic", tt.name)))
		})
	}
	x.put("dynamic/routes.yml", strings.Replace(tiny, "      service: app", "      ruleSyntax: v2\n      service: app", 1))
	read, err = x.readResult()
	require.NoError(t, err)
	require.Equal(t, "text-only", read.Routes[0].Class)
	require.Equal(t, dto.RuleSyntaxV2, read.Routes[0].Eligibility.Reason)
}
func TestInventorySourcesAndWatchGuard(t *testing.T) {
	for _, source := range []string{"cli", "env", "file"} {
		t.Run(source, func(t *testing.T) {
			x := fixture(t)
			switch source {
			case "cli":
				x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--providers.providersThrottleDuration=3s", "--certificatesresolvers.primary.acme.email=operator@example.com", "--experimental.plugins.example.modulename=example.com/plugin")
			case "env":
				x.inspect.Config.Cmd = nil
				x.inspect.Config.Env = []string{"TRAEFIK_PROVIDERS_FILE_DIRECTORY=/etc/traefik/dynamic", "TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=:80/tcp", "TRAEFIK_ENTRYPOINTS_DNS_ADDRESS=:53/udp", "TRAEFIK_PROVIDERS_PROVIDERSTHROTTLEDURATION=3s", "TRAEFIK_CERTIFICATESRESOLVERS_PRIMARY_ACME_EMAIL=operator@example.com", "TRAEFIK_EXPERIMENTAL_PLUGINS_EXAMPLE_MODULENAME=example.com/plugin", "UNRELATED_INPUT=never-return"}
			case "file":
				x.inspect.Config.Cmd = nil
				x.put("traefik.yml", "providers:\n  file:\n    directory: /etc/traefik/dynamic\n    watch: true\n  providersThrottleDuration: 3s\nentryPoints:\n  web:\n    address: ':80'\ncertificatesResolvers:\n  primary:\n    acme: {}\nexperimental:\n  plugins:\n    example:\n      moduleName: example.com/plugin\n")
			}
			inv, err := x.inventory()
			require.NoError(t, err)
			require.True(t, inv.Eligibility.Writable, inv)
			require.Equal(t, int64(3000), inv.ThrottleMillis)
			require.Contains(t, inv.Resolvers, "primary")
			require.Contains(t, inv.Plugins, "example")
			require.Contains(t, inv.Providers, "file")
			require.True(t, inv.Watch)
			b, err := json.Marshal(inv)
			require.NoError(t, err)
			require.NotContains(t, string(b), "operator@example.com")
			require.NotContains(t, string(b), "UNRELATED_INPUT")
		})
	}
	for _, tt := range []struct {
		name, arg string
		want      dto.Reason
	}{
		{"watch", "--providers.file.watch=false", dto.WatchOff},
		{"bad-watch", "--providers.file.watch=unknown", dto.IncompleteInventory},
		{"throttle", "--providers.providersThrottleDuration=bad", dto.IncompleteInventory},
		{"negative-throttle", "--providers.providersThrottleDuration=-1s", dto.IncompleteInventory},
		{"overflow-throttle", "--providers.providersThrottleDuration=2500000h", dto.IncompleteInventory},
		{"port", "--entrypoints.web.address=bad", dto.IncompleteInventory},
		{"zero-port", "--entrypoints.web.address=:0", dto.IncompleteInventory},
		{"high-port", "--entrypoints.web.address=:65536", dto.IncompleteInventory},
		{"outside", "--providers.file.directory=/outside", dto.IncompleteInventory},
		{"both", "--providers.file.filename=/etc/traefik/dynamic/routes.yml", dto.IncompleteInventory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := fixture(t)
			x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, tt.arg)
			inv, err := x.inventory()
			require.NoError(t, err)
			require.False(t, inv.Eligibility.Writable)
			require.Equal(t, tt.want, inv.Eligibility.Reason)
			_, err = x.e.Prepare(t.Context(), x.request("edit-route"))
			requireCode(t, err, tt.want)
		})
	}
	t.Run("static-toml", func(t *testing.T) {
		x := fixture(t)
		x.inspect.Config.Cmd = nil
		x.put("traefik.toml", "[entryPoints.web]\naddress=\":80\"\n")
		inv, err := x.inventory()
		require.NoError(t, err)
		require.False(t, inv.Eligibility.Writable)
		require.Equal(t, dto.UnsupportedFormat, inv.Eligibility.Reason)
	})
}
func TestP25SharedServiceDependantsAndCrossFileGuard(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", shared(t))
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--entrypoints.internal-https.address=:8443")
	r := x.request("edit-service")
	r.Change.RouteID = "dynamic/routes.yml#app-two"
	r.Change.Upstreams = []string{"http://192.0.2.22:8080"}
	in := x.prepare(r)
	require.Equal(t, strings.Replace(shared(t), "http://192.0.2.12:8080", "http://192.0.2.22:8080", 1), string(in.New.Content))
	s, err := state(in)
	require.NoError(t, err)
	require.Len(t, s.Probes, 9)
	for _, p := range s.Probes {
		require.NotEmpty(t, p.URL)
		require.Empty(t, p.Reason)
		require.Contains(t, p.URL, "https://127.0.0.1:")
	}
	routes := map[string]bool{}
	for _, p := range s.Probes {
		routes[p.Route] = true
	}
	require.Len(t, routes, 3)
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), "three.example.com") {
			return response(200, "old")
		}
		return response(200, "changed")
	}
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.NotEqual(t, dto.Verified, obs.State, "one unchanged dependant prevents routing verification")
	x.put("dynamic/other.yml", "http: {routers: {other: {rule: 'Host(`other.example.net`)', service: shared@file}}}\n")
	read, err := x.readResult()
	require.NoError(t, err)
	route, ok := findRoute(read.Routes, r.Change.RouteID, r.Target)
	require.True(t, ok)
	require.Equal(t, "text-only", route.Class)
	require.Equal(t, dto.SharedAcrossFiles, route.Eligibility.Reason)
	_, err = x.attempt(r)
	requireCode(t, err, dto.UnsupportedFormat)
}
func TestCreateEditDeleteAndRouterRename(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Target = "dynamic/new-route.yml"
	r.Expected = dto.FileState{}
	r.Change = dto.Change{Kind: "create-route", Name: "new-route", Rule: "Host(`new.example.net`)", EntryPoints: []string{"web"}, Upstreams: []string{"http://192.0.2.2:8080"}}
	in := x.prepare(r)
	require.False(t, in.Old.Present)
	require.Contains(t, string(in.New.Content), "    new-route:\n")
	require.NoError(t, x.d.Stage(t.Context(), in))
	require.NoError(t, x.validate(in))
	require.NoError(t, x.d.Publish(t.Context(), in))
	require.Equal(t, string(in.New.Content), x.read(r.Target))
	r = x.request("edit-route")
	r.Change.Name = "renamed"
	in = x.prepare(r)
	require.Equal(t, strings.Replace(tiny, "    app:", "    renamed:", 1), string(in.New.Content))
	require.Contains(t, string(in.New.Content), "  services:\n    app:")
	r = x.request("delete-route")
	in = x.prepare(r)
	require.True(t, in.New.Present)
	d, err := parseDocument(string(in.New.Content), "")
	require.NoError(t, err)
	require.Empty(t, keys(at(d.root, "http", "routers")))
	require.NotNil(t, at(d.root, "http", "services", "app"))
}
func TestP27ReferencesAcknowledgedAndPostChecked(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/extra.secret.yml", "opaque")
	r := x.request("edit-route")
	r.Change.Middlewares = []string{"missing@file"}
	_, err := x.attempt(r)
	requireCode(t, err, dto.AcknowledgementRequired)
	r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
	in := x.prepare(r)
	require.NoError(t, x.d.Stage(t.Context(), in))
	require.NoError(t, x.validate(in))
	require.NoError(t, x.d.Publish(t.Context(), in))
	x.probeFunc = func([]string) hostio.ExecResult {
		return hostio.ExecResult{ExitCode: 1, Stderr: []byte("wget: can't connect to remote host\n")}
	}
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.Restore)
	require.Equal(t, dto.ProxyRejected, obs.Reason)
	require.NoError(t, x.d.Restore(t.Context(), in))
	require.Equal(t, tiny, x.read(r.Target))
	// An unchanged reference into an unread file needs no new acknowledgement.
	x.put(r.Target, strings.Replace(tiny, "      service: app", "      middlewares: [private@file]\n      service: app", 1))
	r = x.request("edit-route")
	r.Change.Priority = new(10)
	_, err = x.attempt(r)
	require.NoError(t, err)
}
func TestP30RestoreOwnedThroughNormalProtocol(t *testing.T) {
	x := fixture(t)
	r := x.request("restore-owned")
	x.put(r.Target, "http: [")
	_, err := x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	key := "host\x00" + x.i.Source.Canonical
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", key, r.Target, proxy.Snapshot([]byte(tiny), true).Hash, []byte(tiny)).Error)
	in := x.prepare(r)
	require.Equal(t, tiny, string(in.New.Content))
	require.Equal(t, "http: [", string(in.Old.Content))
	require.NoError(t, x.d.Stage(t.Context(), in))
	require.NoError(t, x.validate(in))
	require.NoError(t, x.d.Publish(t.Context(), in))
	require.Equal(t, tiny, x.read(r.Target))
}
func TestP5CurrentLogErrorsAndStaleErrors(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
	shapes := []string{`ERR Error while building configuration error="field not found" providerName=file`, `INF Router uses a nonexistent resolver routerName=app@file error="resolver missing"`, `{"level":"error","error":"middleware missing","routerName":"app@file"}`, `{"level":"error","providerName":"file","error":"yaml: line 4: did not find expected key"}`, "\x1b[31mERR\x1b[0m Error while parsing rule error=bad routerName=app@file"}
	for _, line := range shapes {
		t.Run(line, func(t *testing.T) {
			x.logs = in.PublishedAt.Format(time.RFC3339Nano) + " " + line + "\n"
			obs, err := x.e.Observe(t.Context(), in, false)
			require.NoError(t, err)
			require.True(t, obs.Restore)
			require.Equal(t, dto.ProxyRejected, obs.Reason)
		})
	}
	x.logs = in.PublishedAt.Add(-time.Second).Format(time.RFC3339Nano) + " " + shapes[0] + "\n"
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, obs.State)
	require.Equal(t, fmt.Sprintf("%d.%09d", in.PublishedAt.Unix(), in.PublishedAt.Nanosecond()), x.logSince[len(x.logSince)-1])
	x.logStatus = 500
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.LogsUnavailable, obs.Reason)
}
func TestP6P7ObservationDeadlineAndFirstResponse(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	initial := x.now
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.NotEqual(t, dto.Verified, obs.State)
	require.True(t, obs.NeedsMarker)
	require.GreaterOrEqual(t, x.now.Sub(initial), 14*time.Second)
	for _, status := range []int{502, 503, 504} {
		x.now = initial
		x.probeFunc = func([]string) hostio.ExecResult { return response(status, "gateway") }
		obs, err = x.e.Observe(t.Context(), in, false)
		require.NoError(t, err)
		require.NotEqual(t, dto.Verified, obs.State)
	}
	x.probeFunc = func([]string) hostio.ExecResult {
		return hostio.ExecResult{Stdout: []byte("different backend"), Stderr: []byte("  HTTP/1.1 301 Moved\n  Location: https://login.example.com/\n  HTTP/1.1 200 OK\n")}
	}
	x.now = initial
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.Routing, obs.Evidence)
	s, err := state(in)
	require.NoError(t, err)
	s.Probes[0].Before = hostio.Response{Status: 301, Headers: map[string]string{"location": "https://login.example.com/"}, Followed: true}
	in.Engine, err = json.Marshal(s)
	require.NoError(t, err)
	x.now = initial
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.NotEqual(t, dto.Verified, obs.State, "a later redirect response cannot prove routing")
	x.now = initial
	x.probeFunc = func([]string) hostio.ExecResult { return response(401, "authentication required") }
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, obs.State)
	default404 := response(404, "404 page not found\n")
	b, err := hostio.ParseProbe(default404, nil)
	require.NoError(t, err)
	a, err := hostio.ParseProbe(response(404, "backend not found"), nil)
	require.NoError(t, err)
	// BusyBox shows a backend 404 exactly like Traefik's no-route answer.
	require.False(t, routingEvidence("create-route", b, a))
	answered, err := hostio.ParseProbe(response(200, "fresh"), nil)
	require.NoError(t, err)
	require.True(t, routingEvidence("create-route", b, answered))
	require.False(t, routingEvidence("create-route", hostio.Response{Status: 200}, a))
	require.False(t, routingEvidence("delete-route", b, a))
	require.True(t, routingEvidence("delete-route", answered, b))
	require.False(t, routingEvidence("delete-route", a, b), "a bare 404 before a delete is already no route")
}
func TestP29MarkerLoadAndFrameworkCleanup(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Resolver = "primary"
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.primary.acme.email=operator@example.com")
	in := x.prepare(r)
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.NeedsMarker)
	require.NoError(t, x.e.Mark(t.Context(), in))
	marker := x.read(in.MarkerPath)
	require.Contains(t, marker, "http://127.0.0.1:1")
	require.Contains(t, marker, "Host(`example-op.docklet-check.invalid`)")
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), ".docklet-check.invalid") {
			return response(502, "closed")
		}
		return response(200, "old")
	}
	obs, err = x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, obs.State)
	require.Equal(t, dto.Load, obs.Evidence)
	require.NoError(t, x.e.Cleanup(t.Context(), in))
	require.NoError(t, x.d.Cleanup(t.Context(), in))
	_, err = os.Stat(filepath.Join(x.root, in.MarkerPath))
	require.ErrorIs(t, err, os.ErrNotExist)
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.websecure.address=:443", "--entrypoints.websecure.http.tls=true"}
	r = x.request("edit-route")
	r.Change.Priority = new(10)
	in = x.prepare(r)
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, obs.State)
	require.Equal(t, dto.NotObservable, obs.Reason)
}

func TestRealProbeRoundtrip(t *testing.T) {
	x := fixture(t)
	res, err := x.d.Exec(t.Context(), x.i, "probe", "", "app.example.com", "http://127.0.0.1:80/")
	require.NoError(t, err)
	require.Equal(t, response(200, "old"), res)
	p, err := hostio.ParseProbe(res, err)
	require.NoError(t, err)
	require.Equal(t, 200, p.Status)
}

func TestAgentAutomaticallyRestoresRejectedRouteAndCleansMarker(t *testing.T) {
	for _, marked := range []bool{false, true} {
		t.Run(fmt.Sprint(marked), func(t *testing.T) {
			x := fixture(t)
			x.now = time.Now()
			a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
			require.NoError(t, err)
			r := x.request("edit-route")
			r.Generation = a.Generation()
			r.Instance.Generation = r.Generation
			r.Instance.Registration = r.Registration
			if marked {
				x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.primary.acme.email=operator@example.com")
				r.Change.Resolver = "primary"
				x.probeFunc = func(args []string) hostio.ExecResult {
					if _, exists := os.Stat(filepath.Join(x.root, "dynamic/docklet-check-"+r.ID+".yml")); strings.Contains(strings.Join(args, " "), "docklet-check.invalid") && exists == nil {
						x.logs = "CURRENT " + ` {"level":"error","error":"bad rule","routerName":"app@file"}` + "\n"
						return response(502, "closed")
					}
					return response(200, "old")
				}
			} else {
				r.Change.Middlewares = []string{"missing@file"}
				r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
				x.logs = "CURRENT " + ` {"level":"error","error":"middleware missing","routerName":"app@file"}` + "\n"
			}
			op, err := a.Apply(t.Context(), r)
			require.NoError(t, err)
			require.Equal(t, dto.Restored, op.State)
			require.Equal(t, dto.ProxyRejected, op.Reason)
			require.Equal(t, tiny, x.read(r.Target))
			files, err := os.ReadDir(filepath.Join(x.root, "dynamic"))
			require.NoError(t, err)
			require.Len(t, files, 1)
			require.Equal(t, "routes.yml", files[0].Name())
		})
	}
}
func TestDelayedRoutingWaitsForActualEffect(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	var effect atomic.Bool
	waited := false
	x.e.wait = func(_ context.Context, d time.Duration) error {
		require.False(t, effect.Load(), "first unchanged response cannot verify")
		waited = true
		effect.Store(true)
		x.now = x.now.Add(d)
		return nil
	}
	x.probeFunc = func([]string) hostio.ExecResult {
		if effect.Load() {
			return response(200, "new")
		}
		return response(200, "old")
	}
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, waited)
	require.Equal(t, dto.Verified, obs.State)
}

func TestMarkerWithoutAnswerAndUnchangedCatchAllIsUnverified(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Priority = new(10)
	in := x.prepare(r)
	require.NoError(t, x.e.Mark(t.Context(), in))
	for _, res := range []hostio.ExecResult{{ExitCode: 1, Stderr: []byte("no connection")}, response(404, "404 page not found\n"), response(200, "old")} {
		x.probeFunc = func([]string) hostio.ExecResult { return res }
		obs, err := x.e.Observe(t.Context(), in, true)
		require.NoError(t, err)
		require.Equal(t, dto.AppliedUnverified, obs.State)
		require.Equal(t, dto.NotObservable, obs.Reason)
	}
}
func TestPrepareInvalidRequestsRefuseWithoutEffects(t *testing.T) {
	x := fixture(t)
	for _, change := range []dto.Change{{Kind: "unknown"}, {Kind: "create-route", Name: "bad/name", Rule: "Host(`a.example.com`)", Upstreams: []string{"http://192.0.2.1"}}, {Kind: "edit-route", RouteID: "missing"}, {Kind: "edit-route", RouteID: "app", Name: "bad/name"}, {Kind: "edit-route", RouteID: "app", Rule: "Unknown(`x`)"}, {Kind: "edit-route", RouteID: "app", Upstreams: []string{}}, {Kind: "edit-route", RouteID: "app", Resolver: "new-resolver"}, {Kind: "edit-route", RouteID: "app", Upstreams: []string{"file:///etc/example"}}} {
		r := x.request("edit-route")
		r.Change = change
		require.NotPanics(t, func() { _, err := x.attempt(r); require.Error(t, err) })
		require.Equal(t, tiny, x.read(r.Target))
	}
	for _, target := range []string{"../routes.yml", "dynamic/auth.secret.yml", "dynamic/config.toml", "outside/routes.yml", "dynamic/data.txt"} {
		r := x.request("edit-route")
		r.Target = target
		_, err := x.attempt(r)
		require.Error(t, err)
	}
}
func TestLabelRoutesLinkOnlyToManagedStack(t *testing.T) {
	x := fixture(t)
	r, err := x.readResult()
	require.NoError(t, err)
	require.Equal(t, "label-read-only", r.Routes[len(r.Routes)-1].Class)
	require.Empty(t, r.Routes[len(r.Routes)-1].ProjectID)
	require.NoError(t, x.db.Exec("INSERT INTO projects(id,name,path,status,is_archived) VALUES(?,?,?,?,?)", "example-id", "example-stack", "/example/projects/example-stack", "running", false).Error)
	r, err = x.readResult()
	require.NoError(t, err)
	route := r.Routes[len(r.Routes)-1]
	require.Equal(t, "example-id", route.ProjectID)
	require.Equal(t, "example-app", route.ContainerName)
}

func TestInventoryIncompleteAndCaseSensitiveNames(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = []string{"--providers.file.directory", "/etc/traefik/dynamic", "--entrypoints.webSecure.address=:443", "--entrypoints.webSecure.http.tls=false", "--certificatesresolvers.DnsPrimary.acme.email=operator@example.com", "--providers.docker"}
	x.inspect.Config.Entrypoint = []string{"traefik"}
	v, err := x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
	require.Equal(t, "webSecure", v.EntryPoints[0].Name)
	require.False(t, v.EntryPoints[0].TLS)
	require.Equal(t, []string{"DnsPrimary"}, v.Resolvers)
	x.inspect.Config = nil
	v, err = x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
	x.i.Identity.ContainerID = "other"
	_, err = x.inventory()
	require.Error(t, err)
}
func TestInvalidIntentAndMarkerGuards(t *testing.T) {
	x := fixture(t)
	in := proxy.Intent{Request: x.request("edit-route"), Engine: []byte("invalid")}
	_, err := x.e.Observe(t.Context(), in, false)
	require.Error(t, err)
	require.Error(t, x.e.Mark(t.Context(), in))
	in = x.prepare(x.request("edit-route"))
	in.MarkerPath = ""
	require.Error(t, x.e.Mark(t.Context(), in))
	in.MarkerPath = "dynamic/wrong.yml"
	require.Error(t, x.e.Mark(t.Context(), in))
	in.MarkerPath = "dynamic/docklet-check-example-op.yml"
	in.Request.ID = "bad/name"
	require.Error(t, x.e.Mark(t.Context(), in))
	require.Error(t, x.e.Restart(t.Context(), in))
	require.NoError(t, x.e.Reload(t.Context(), in))
}

func TestRecoveryFreshDeadlineAndOpaqueHealthEpoch(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Priority = new(10)
	in := x.prepare(r)
	in.PublishedAt = x.now.Add(-time.Hour)
	start := x.now
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.NeedsMarker)
	require.GreaterOrEqual(t, x.now.Sub(start), 14*time.Second)
	x.put("dynamic/extra.secret.yml", "opaque")
	x.inspect.State.StartedAt = epoch.Add(time.Minute).Format(time.RFC3339Nano)
	x.i.Identity.StartedAt = x.inspect.State.StartedAt
	x.logs = epoch.Add(time.Second).Format(time.RFC3339Nano) + ` ERR configuration error="invalid yaml" providerName=file` + "\n"
	_, err = x.e.Prepare(t.Context(), x.request("edit-route"))
	require.NoError(t, err, "a provider error from the previous epoch cannot block opaque writes")
}
