// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine tests against measured producer output.

package traefik

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestMeasuredNoRouteAnswerIsRecognised(t *testing.T) {
	r, err := hostio.ParseProbe(response(404, ""), nil)
	require.NoError(t, err)
	require.True(t, noRoute(r), "BusyBox reports Traefik's no-route answer as a bare 404")
	r, err = hostio.ParseProbe(response(200, "body"), nil)
	require.NoError(t, err)
	require.False(t, noRoute(r))
}

func TestMeasuredNoRouteRestoresAnAcknowledgedUnresolvedReference(t *testing.T) {
	x := fixture(t)
	x.now = time.Now()
	a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	r := x.request("edit-route")
	r.Generation = a.Generation()
	r.Instance.Generation = r.Generation
	r.Instance.Registration = r.Registration
	r.Change.Middlewares = []string{"missing@file"}
	r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
	x.probeFunc = func([]string) hostio.ExecResult {
		b, err := os.ReadFile(filepath.Join(x.root, r.Target))
		if err == nil && strings.Contains(string(b), "missing@file") {
			return response(404, "")
		}
		return response(200, "old")
	}
	op, err := a.Apply(t.Context(), r)
	require.NoError(t, err)
	require.Equal(t, dto.Restored, op.State, "a router that no longer answers restores (P27)")
}

func TestMeasuredNoRouteBeforeACreateGivesRoutingEvidence(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Target = "dynamic/new.yml"
	r.Expected = dto.FileState{}
	r.Change = dto.Change{Kind: "create-route", Name: "fresh", Rule: "Host(`fresh.example.com`)", EntryPoints: []string{"web"}, Upstreams: []string{"http://192.0.2.5:80"}}
	created := false
	x.probeFunc = func([]string) hostio.ExecResult {
		if created {
			return response(200, "fresh")
		}
		return response(404, "")
	}
	in := x.prepare(r)
	created = true
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, obs.State)
	require.Equal(t, dto.Routing, obs.Evidence)
}

func TestUnrelatedACMEFailureNeitherRestoresNorRefuses(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
	x.logs = in.PublishedAt.Format(time.RFC3339Nano) + " " + measuredACME + "\n"
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.False(t, obs.Restore, "another provider's record never rejects the change: %+v", obs)

	y := fixture(t)
	y.put("dynamic/extra.secret.yml", "opaque")
	y.logs = epoch.Add(time.Minute).Format(time.RFC3339Nano) + " " + measuredACME + "\n"
	r = y.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	_, err = y.attempt(r)
	require.NoError(t, err, "the opaque health check ignores ACME issuance failures")

	z := fixture(t)
	z.put("dynamic/extra.secret.yml", "opaque")
	z.logs = epoch.Add(time.Minute).Format(time.RFC3339Nano) + ` {"level":"error","routerName":"app@file","error":"bad"}` + "\n"
	r = z.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	_, err = z.attempt(r)
	require.NoError(t, err, "the opaque health check counts aggregate file-provider errors only, not router records")
}

func TestTargetRouterErrorRestoresTheChange(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Rule = "Host(`next.example.com`)"
	in := x.prepare(r)
	s, err := state(in)
	require.NoError(t, err)
	require.NotEmpty(t, s.Scope)
	x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
	x.logs = in.PublishedAt.Format(time.RFC3339Nano) + ` {"level":"error","routerName":"` + s.Scope[0] + `","error":"bad"}` + "\n"
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.Restore, "a record naming the target's router rejects the change")
	require.Equal(t, dto.ProxyRejected, obs.Reason)
}

func TestStaticFinderPrefersTOMLAndFlagsOnly(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = nil
	x.inspect.Config.Entrypoint = []string{"/entrypoint.sh", "traefik"}
	x.put("traefik.yml", "entryPoints:\n  web:\n    address: ':80'\nproviders:\n  file:\n    directory: /etc/traefik/dynamic\n")
	x.put("traefik.toml", "[entryPoints.web]\n  address = \":80\"\n")
	v, err := x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable, "Traefik's finder prefers traefik.toml: %+v", v.Eligibility)
	require.Equal(t, "/etc/traefik/traefik.toml", v.RootFile)

	y := fixture(t)
	y.inspect.Config.Cmd = nil
	y.inspect.Config.Env = []string{"TRAEFIK_CONFIGFILE=/etc/traefik/chosen.yml", "TRAEFIK_PROVIDERS_FILE_DIRECTORY=/etc/traefik/dynamic", "TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=:80"}
	y.put("chosen.yml", "entryPoints:\n  web:\n    address: ':80'\nproviders:\n  file:\n    directory: /etc/traefik/dynamic\n")
	v, err = y.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable, "Traefik ignores an environment configuration file location")
	require.Equal(t, dto.IncompleteInventory, v.Eligibility.Reason)
}

func TestStaticOptionsFollowTraefikTypes(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--providers.providersThrottleDuration=10", "--experimental.localPlugins.example.moduleName=github.com/example/plugin")
	v, err := x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable, "%+v", v.Eligibility)
	require.Equal(t, int64(10000), v.ThrottleMillis, "a plain integer duration is seconds")
	require.Equal(t, []string{"example"}, v.Plugins)
}

func TestEmptyDynamicFileIsAnEmptyConfiguration(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/empty.yml", "# placeholder\n")
	x.put("dynamic/zero.yml", "")
	read, err := x.readResult()
	require.NoError(t, err)
	require.True(t, read.Eligibility.Writable, "%+v", read.Eligibility)
}

func TestMatcherArgumentsAndSpellingsFollowTraefik(t *testing.T) {
	for _, rule := range []string{"Host(``)", "Method(``)", "Header(`X-A`, ``)", "Query(``)", "PathRegexp(``)"} {
		_, err := parseRule(rule)
		require.Error(t, err, "empty args for matcher: %s", rule)
	}
	for _, rule := range []string{"host(`a.example.com`)", "HOST(`a.example.com`)", "Pathprefix(`/a`)", "pathprefix(`/a`) && Host(`a.example.com`)"} {
		calls, err := parseRule(rule)
		require.NoError(t, err, rule)
		require.Contains(t, []string{"Host", "PathPrefix"}, calls[0].name, rule)
	}
	_, err := parseRule("hOsT(`a.example.com`)")
	require.Error(t, err, "mixed case is not a registered spelling")
}

func TestSubdirectoryFilesAreNotWritable(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/sub/nested.yml", strings.Replace(tiny, "app", "nested", -1))
	read, err := x.readResult()
	require.NoError(t, err)
	found := false
	for _, r := range read.Routes {
		if r.File == "dynamic/sub/nested.yml" {
			found = true
			require.Equal(t, "text-only", r.Class)
			require.Equal(t, dto.WatchOff, r.Eligibility.Reason)
		}
	}
	require.True(t, found)
	r := x.request("create-route")
	r.Target = "dynamic/sub/new.yml"
	r.Expected = dto.FileState{}
	r.Change = dto.Change{Kind: "create-route", Name: "fresh", Rule: "Host(`fresh.example.com`)", EntryPoints: []string{"web"}, Upstreams: []string{"http://192.0.2.5:80"}}
	_, err = x.attempt(r)
	requireCode(t, err, dto.WatchOff)
}

func TestLogScopeComparesNamesByteForByte(t *testing.T) {
	for _, tt := range []struct {
		line  string
		scope string
		want  bool
	}{
		{`{"level":"error","routerName":"App@file","error":"bad"}`, "App@file", true},
		{`ERR error="bad" routerName=App@file`, "App@file", true},
		{`{"level":"error","serviceName":"Svc@file","error":"bad"}`, "Svc@file", true},
		{`ERR error="bad" middlewareName=Mw@file`, "Mw@file", true},
		{`{"level":"error","routerName":"App@file","error":"bad"}`, "app@file", false},
		{`{"level":"error","serviceName":"svc@file","error":"bad"}`, "Svc@file", false},
		{`ERR error="bad" middlewareName=MW@file`, "Mw@file", false},
		{`ERR rejected routerName=MYapp@file`, "app@file", false},
	} {
		require.Equal(t, tt.want, providerError(tt.line, []string{tt.scope}), "%s / %s", tt.line, tt.scope)
	}
}

func TestReadSummarisesTLSAndListsMiddlewareDefinitions(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.primary.acme.email=operator@example.com")
	x.put("dynamic/routes.yml", strings.Replace(tiny, "      service: app\n", "      service: app\n      tls:\n        certResolver: primary\n", 1))
	x.put("dynamic/plain.yml", strings.Replace(strings.ReplaceAll(tiny, "app", "plain"), "      service: plain\n", "      service: plain\n      tls:\n", 1))
	x.put("dynamic/mw.yml", "http:\n  middlewares:\n    zeta:\n      headers: {frameDeny: true}\n    alpha:\n      headers: {frameDeny: true}\n")
	read, err := x.readResult()
	require.NoError(t, err)
	app, ok := findRoute(read.Routes, "dynamic/routes.yml#app", "dynamic/routes.yml")
	require.True(t, ok)
	require.True(t, app.TLS)
	require.Equal(t, "primary", app.Resolver)
	plain, ok := findRoute(read.Routes, "dynamic/plain.yml#plain", "dynamic/plain.yml")
	require.True(t, ok)
	require.False(t, plain.TLS, "an empty tls key is a null section")
	require.Empty(t, plain.Resolver)
	require.Equal(t, []dto.Reference{{Name: "zeta", Kind: "local", File: "dynamic/mw.yml"}, {Name: "alpha", Kind: "local", File: "dynamic/mw.yml"}}, read.Middlewares, "paths sorted, definitions in document order")
}

func TestResolverChangeKeepsTLSDomains(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.primary.acme.email=operator@example.com", "--certificatesresolvers.secondary.acme.email=operator@example.com")
	x.put("dynamic/routes.yml", strings.Replace(tiny, "      service: app\n", "      service: app\n      tls:\n        certResolver: primary\n        domains:\n          - main: example.com # Base\n            sans: ['*.example.com']\n", 1))
	r := x.request("edit-route")
	r.Change.Resolver = "secondary"
	in := x.prepare(r)
	text := string(in.New.Content)
	require.Contains(t, text, "certResolver: secondary")
	require.Contains(t, text, "          - main: example.com # Base\n            sans: ['*.example.com']\n", "domains and their comment stay byte for byte")
	require.NotContains(t, text, "primary")
}

// Early smoke 3: Traefik v3.7.16 rejects `routers:` followed by an empty mapping
// ("routers cannot be a standalone element"); deleting the last router drops the section.
func TestDeletingTheLastRouterDropsTheRoutersSection(t *testing.T) {
	x := fixture(t)
	r := x.request("delete-route")
	in := x.prepare(r)
	text := string(in.New.Content)
	require.NotContains(t, text, "routers")
	require.NotContains(t, text, "{}")
	require.Contains(t, text, "  services:\n    app:\n", "the service and the file stay")
	// A router whose service lives in an opaque file stays editable (kickoff); its file may hold only routers.
	x.put("dynamic/svc.secret.yml", "opaque")
	x.put("dynamic/only.yml", "http:\n  routers:\n    solo:\n      rule: Host(`solo.example.com`)\n      service: hidden@file\n")
	r = x.request("delete-route")
	r.Target, r.Change.RouteID = "dynamic/only.yml", "dynamic/only.yml#solo"
	b, err := x.d.Target(t.Context(), x.i, r.Target)
	require.NoError(t, err)
	r.Expected = b.FileState
	r.GraphHashes, err = x.d.Graph(t.Context(), x.i)
	require.NoError(t, err)
	in = x.prepare(r)
	require.NotContains(t, string(in.New.Content), "http", "an emptied http section is dropped too")
}
