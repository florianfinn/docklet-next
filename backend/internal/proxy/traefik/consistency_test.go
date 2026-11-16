// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik consistency regressions.

package traefik

import (
	"context"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	"github.com/moby/moby/api/types/container"
	"strconv"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
	"gorm.io/gorm"
)

func TestDeadlinePreservesTheSpecificCapabilityReason(t *testing.T) {
	for _, tt := range []struct {
		marked             bool
		marker, last, want dto.Reason
	}{
		{false, dto.ProbeOptionUnavailable, dto.NoObservableDifference, dto.ProbeOptionUnavailable},
		{true, "", dto.ProbeOptionUnavailable, dto.ProbeOptionUnavailable},
		{true, "", dto.NoObservableDifference, dto.NotObservable},
		{false, "", dto.NoObservableDifference, ""},
	} {
		got := deadlineObservation(engineState{Marker: probe{Reason: tt.marker}}, tt.marked, tt.last, false)
		require.Equal(t, tt.want, got.Reason)
		require.Equal(t, !tt.marked && tt.marker == "", got.NeedsMarker)
	}
}

func TestRuleLimitsAndNestedErrorPropagation(t *testing.T) {
	for _, rule := range []string{
		"Host(`app.example.com`)" + strings.Repeat(" ", 16385),
		"Host(`app.example.com`) || Unknown(`x`)",
		"Host(`app.example.com`) && Unknown(`x`)",
		"(Host(`app.example.com`) || Unknown(`x`))",
		"(Host(`app.example.com`) && Unknown(`x`))",
		`Header("X-Example", "\q")`,
	} {
		require.NotPanics(t, func() { _, err := parseRule(rule); require.Error(t, err, rule) })
	}
	for _, rule := range []string{`Header("X-Example", "a\"b")`, "Header(`X-Example`, `a\\b`)", `Header("X-Example", "a\\b")`} {
		_, err := parseRule(rule)
		require.NoError(t, err, rule)
	}
}

func TestClassifierRetainsEveryReferenceKindAndInfrastructureRule(t *testing.T) {
	for _, rule := range []string{"PathPrefix(`/`)", `PathPrefix("/")`, "HostRegexp(`.*`)", `HostRegexp(".*")`, "HostRegexp(`^.*$`)", `HostRegexp("^.*$")`, "Host(`*`)", `Host("*")`} {
		d, err := parseDocument(strings.Replace(tiny, "Host(`app.example.com`)", rule, 1), "")
		require.NoError(t, err)
		r := classify(map[string]*document{"routes.yml": d}, nil, dto.Inventory{})
		require.Len(t, r, 1)
		require.Equal(t, "infrastructure", r[0].Class, rule)
	}
	for _, kind := range []string{"redirectScheme: {scheme: https}", "redirectRegex: {regex: '^http://', replacement: 'https://'}"} {
		src := strings.Replace(tiny, "      service: app", "      middlewares: [redirect]\n      service: app", 1) + "  middlewares:\n    redirect:\n      " + kind + "\n"
		d, err := parseDocument(src, "")
		require.NoError(t, err)
		require.Equal(t, "infrastructure", classify(map[string]*document{"routes.yml": d}, nil, dto.Inventory{})[0].Class)
	}
	d, err := parseDocument(tiny, "routes.yml")
	require.NoError(t, err)
	docs := map[string]*document{"routes.yml": d}
	require.Equal(t, "internal", resolve(docs, nil, dto.Inventory{}, "services", "api@internal").Kind)
	require.Equal(t, "unresolved", resolve(docs, nil, dto.Inventory{}, "services", "missing@docker").Kind)
	require.Equal(t, "external", resolve(docs, nil, dto.Inventory{Providers: []string{"docker"}}, "services", "missing@docker").Kind)
	require.Equal(t, "unresolved", resolve(docs, nil, dto.Inventory{}, "services", "missing").Kind)
	ref := resolve(docs, []dto.OpaqueFile{{Path: "one.secret.yml"}, {Path: "two.secret.yml"}}, dto.Inventory{}, "services", "missing")
	require.Equal(t, "opaque", ref.Kind)
	require.Empty(t, ref.File)
	for _, tt := range []struct{ from, to string }{
		{"service: app", "service: unavailable"},
		{"loadBalancer:\n        servers: [{url: 'http://192.0.2.1:8080'}]", "loadBalancer: {}"},
		{"servers: [{url: 'http://192.0.2.1:8080'}]", "servers: []"},
		{"servers: [{url: 'http://192.0.2.1:8080'}]", "servers: {}"},
		{"servers: [{url: 'http://192.0.2.1:8080'}]", "servers: [{url: 'ftp://example.com'}]"},
		{"loadBalancer:", "loadBalancer:\n        passHostHeader: false"},
		{"loadBalancer:\n        servers: [{url: 'http://192.0.2.1:8080'}]", "weighted: {services: [{name: other}]}"},
		{"rule: Host(`app.example.com`)", "rule: Unknown(`x`)"},
	} {
		doc, err := parseDocument(strings.Replace(tiny, tt.from, tt.to, 1), "")
		require.NoError(t, err)
		require.NotPanics(t, func() {
			routes := classify(map[string]*document{"routes.yml": doc}, nil, dto.Inventory{})
			require.Len(t, routes, 1)
			require.Equal(t, "text-only", routes[0].Class, tt.to)
		})
	}
	doc, err := parseDocument(strings.Replace(tiny, "      service: app", "      priority: 10\n      service: app", 1), "")
	require.NoError(t, err)
	require.Equal(t, 10, classify(map[string]*document{"routes.yml": doc}, nil, dto.Inventory{})[0].Priority)
}

func TestReferenceGraphIncludesTLSAndCompositeServices(t *testing.T) {
	source := `http:
  routers:
    app:
      rule: Host("app.example.com")
      service: app
      middlewares: [chain, missing]
      entryPoints: [web]
      tls: {options: strict, certResolver: production}
  services:
    app: {mirroring: {service: primary, mirrors: [{name: mirror}]}}
    fail: {failover: {service: first, fallback: fallback}}
    weighted: {weighted: {services: [{name: balanced}]}}
  middlewares:
    chain: {chain: {middlewares: [missing]}}
tls:
  options: {strict: {minVersion: VersionTLS12}}
`
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	require.NoError(t, validateDocument(d, "routes.yml"))
	docs := map[string]*document{"routes.yml": d}
	inv := dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "web"}}, Resolvers: []string{"production"}}
	in := proxy.Intent{Request: dto.ApplyRequest{Target: "routes.yml"}}
	names, err := newUnresolved(docs, nil, inv, in)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"missing", "primary", "mirror", "first", "fallback", "balanced"}, names)
	_, err = newUnresolved(docs, nil, dto.Inventory{}, in)
	requireCode(t, err, dto.InvalidGraph)
	d, err = parseDocument(strings.Replace(source, "options: strict", "options: default", 1), "")
	require.NoError(t, err)
	names, err = newUnresolved(map[string]*document{"routes.yml": d}, nil, inv, in)
	require.NoError(t, err)
	require.NotContains(t, names, "default")
	in.Old = proxy.Snapshot([]byte(source), true)
	names, err = newUnresolved(docs, nil, inv, in)
	require.NoError(t, err)
	require.Empty(t, names)
	in.Request.Target = "other.yml"
	names, err = newUnresolved(docs, nil, inv, in)
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestNonStringKeysAndUnsupportedScalarSourcesStayReadOnly(t *testing.T) {
	for _, source := range []string{"http: {routers: {123: {rule: 'Host(`app.example.com`)', service: app}}}", "http: {? [a,b]: {}}", "http:\n  routers:\n    app:\n      rule: >\n        Host(`app.example.com`)\n", "http:\n  routers:\n    app:\n      rule: |\n        Host(`app.example.com`)\n"} {
		require.NotPanics(t, func() { _, err := parseDocument(source, "routes.yml"); require.Error(t, err) })
	}
}

func TestCreateAdmissionAndRenameFailuresKeepTheirReason(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Change = dto.Change{Kind: "create-route", Name: "new", Rule: "Host(`new.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
	_, err := x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	r.Target = "dynamic/new.yml"
	r.Change.Name = "bad/name"
	_, err = x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	r.Change.Name = "new"
	r.Change.Upstreams = nil
	_, err = x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	r.Change.Upstreams = []string{"http://192.0.2.2:8080"}
	r.Change.Service = "bad/service"
	_, err = x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	r = x.request("edit-route")
	r.Target = "dynamic/missing.yml"
	_, err = x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	r = x.request("edit-route")
	r.Change.Name = "app"
	r.Change.Priority = new(10)
	_, err = x.attempt(r)
	require.NoError(t, err, "retaining the existing name is not a collision")
	r.Change.Service = "app"
	r.Change.Upstreams = []string{"http://192.0.2.2:8080"}
	in, err := x.attempt(r)
	require.NoError(t, err)
	require.Contains(t, string(in.New.Content), "http://192.0.2.2:8080")
	x.put(r.Target, strings.Replace(tiny, "  routers:\n", "  routers:\n    second:\n      rule: Host(`second.example.com`)\n      service: app\n", 1))
	r.Change.Name = "second"
	r.Change.Upstreams = nil
	_, err = x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
}

func TestRestoreRequiresVerifiedContentAndWritableInventory(t *testing.T) {
	x := fixture(t)
	r := x.request("restore-owned")
	key := "host\x00" + x.i.Source.Canonical
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", key, r.Target, "", []byte{}).Error)
	_, err := x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
	require.NoError(t, x.db.Exec("UPDATE proxy_verified_files SET hash=?,content=? WHERE source=? AND path=?", proxy.Snapshot([]byte(tiny), true).Hash, []byte(tiny), key, r.Target).Error)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--providers.file.watch=false")
	_, err = x.attempt(r)
	requireCode(t, err, dto.WatchOff)
}

func TestMarkerRefusesInvalidStateAndExistingTemporaryBytes(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	bad := in
	bad.Engine = []byte("invalid")
	require.Error(t, x.e.Mark(t.Context(), bad))
	s, err := state(in)
	require.NoError(t, err)
	s.Marker.Reason = dto.NotObservable
	bad = in
	bad.Engine, err = json.Marshal(s)
	require.NoError(t, err)
	requireCode(t, x.e.Mark(t.Context(), bad), dto.NotObservable)
	bad = in
	bad.Request.ID = "bad/name"
	bad.MarkerPath = "dynamic/docklet-check-bad/name.yml"
	requireCode(t, x.e.Mark(t.Context(), bad), dto.InvalidInput)
	x.put(in.TempPath, "stale bytes")
	require.ErrorIs(t, x.e.Mark(t.Context(), in), os.ErrExist)
	require.Equal(t, "stale bytes", x.read(in.TempPath))
	_, err = os.Stat(filepath.Join(x.root, in.MarkerPath))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestAcknowledgedUnobservableAndFailedProbesRestore(t *testing.T) {
	for _, noHost := range []bool{false, true} {
		x := fixture(t)
		r := x.request("edit-route")
		r.Change.Middlewares = []string{"missing@file"}
		r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
		if noHost {
			r.Change.Rule = "PathPrefix(`/api`)"
		}
		in := x.prepare(r)
		x.probeFunc = func([]string) hostio.ExecResult {
			return hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}
		}
		got, err := x.e.Observe(t.Context(), in, false)
		require.NoError(t, err)
		require.True(t, got.Restore)
		require.Equal(t, dto.ProxyRejected, got.Reason)
	}
}

func TestMarkerCannotBorrowLateRoutingEvidence(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	require.NoError(t, x.e.Mark(t.Context(), in))
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
			return response(404, "404 page not found\n")
		}
		return response(200, "late routing effect")
	}
	got, err := x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, got.State)
	require.Equal(t, dto.NotObservable, got.Reason)
}

func TestParentCancellationDuringProbeAndWaitIsReturned(t *testing.T) {
	for _, duringProbe := range []bool{false, true} {
		x := fixture(t)
		in := x.prepare(x.request("edit-route"))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		if duringProbe {
			x.probeFunc = func([]string) hostio.ExecResult { cancel(); return response(200, "changed") }
		} else {
			x.e.wait = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
		}
		_, err := x.e.Observe(ctx, in, false)
		require.ErrorIs(t, err, context.Canceled)
	}
}

func TestReadKeepsOwnershipAndConsumesHiddenFiles(t *testing.T) {
	x := fixture(t)
	old := proxy.Snapshot([]byte(strings.Replace(tiny, "app.example.com", "old.example.com", 1)), true)
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", "host\x00"+x.i.Source.Canonical, "dynamic/routes.yml", old.Hash, old.Content).Error)
	x.put("dynamic/.docklet-op.tmp", "invalid: [")
	read, err := x.readResult()
	require.NoError(t, err)
	require.True(t, read.Eligibility.Writable)
	require.Len(t, read.Files, 1)
	require.True(t, read.Files[0].Owned)
	require.Equal(t, old.Hash, read.Files[0].LastVerifiedHash)
	// Traefik's loader reads hidden files and directories; one invalid file blocks the directory.
	for _, hidden := range []string{"dynamic/.hidden.yml", "dynamic/.hidden/nested.yml"} {
		y := fixture(t)
		y.put(hidden, "invalid: [")
		read, err = y.readResult()
		require.NoError(t, err)
		require.False(t, read.Eligibility.Writable, hidden)
		require.Equal(t, hidden, read.Eligibility.Path)
		r := y.request("edit-route")
		r.Change.Rule = "Host(`next.example.com`)"
		_, err = y.attempt(r)
		require.Error(t, err, hidden)
	}
	z := fixture(t)
	z.put("dynamic/.legacy.toml", "[http.routers]\n")
	read, err = z.readResult()
	require.NoError(t, err)
	require.False(t, read.Eligibility.Writable, "a hidden consumed TOML file makes the instance read-only")
	x.put("dynamic/duplicate.yml", tiny)
	read, err = x.readResult()
	require.NoError(t, err)
	require.Equal(t, dto.InvalidGraph, read.Eligibility.Reason)
	require.Contains(t, []string{"dynamic/routes.yml", "dynamic/duplicate.yml"}, read.Eligibility.Path)
}

func TestReadDirectoryCannotEscapeTheRegisteredMount(t *testing.T) {
	x := fixture(t)
	x.i.RouteDirectory = "/outside"
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/outside", "--entrypoints.web.address=:80"}
	_, err := x.readResult()
	requireCode(t, err, dto.InvalidInput)
	require.NotContains(t, x.archives, "/outside")
}

func TestDockerLabelSelectorsAndListErrorsUseTheRealTransport(t *testing.T) {
	x := fixture(t)
	x.containers = []container.Summary{{ID: "example-labels", Labels: map[string]string{"traefik.http.routers": "short", "traefik.http.routers.short": "short", "other.http.routers.fake.rule": "Host(`wrong.example.com`)", "traefik.http.routers.valid.rule": "Host(`valid.example.com`)"}}}
	read, err := x.readResult()
	require.NoError(t, err)
	require.Len(t, read.Routes, 2)
	require.Equal(t, "valid", read.Routes[1].Name)
	x.containerStatus = 503
	_, err = x.readResult()
	require.ErrorContains(t, err, "list unavailable")
}

func TestOpaqueHealthEpochUsesConsumedHeaderTimesOnly(t *testing.T) {
	for _, changed := range []string{"routes.yml", "auth.secret.txt"} {
		x := fixture(t)
		x.put("dynamic/auth.secret.txt", "opaque")
		x.put("dynamic/notes.txt", "ignored")
		update := epoch.Add(20 * time.Minute)
		x.archiveModified = map[string]time.Time{"dynamic/" + changed: update, "dynamic/notes.txt": epoch.Add(24 * time.Hour)}
		x.logs = update.Add(-time.Second).Format(time.RFC3339Nano) + ` ERR error="old" providerName=file` + "\n"
		r := x.request("edit-route")
		r.Change.Priority = new(10)
		_, err := x.attempt(r)
		require.NoError(t, err)
		require.NotEmpty(t, x.logSince)
		require.Equal(t, strconv.FormatInt(update.Unix(), 10)+".000000000", x.logSince[0])
	}
	x := fixture(t)
	x.put("dynamic/auth.secret.yml", "opaque")
	x.logs = epoch.Add(time.Minute).Format(time.RFC3339Nano) + ` ERR error="current" providerName=file` + "\n"
	_, err := x.attempt(x.request("edit-route"))
	requireCode(t, err, dto.ProxyRejected)
}

func TestRestoreStillRejectsInvalidSiblingsAndDuplicateDefinitions(t *testing.T) {
	x := fixture(t)
	r := x.request("restore-owned")
	key := "host\x00" + x.i.Source.Canonical
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", key, r.Target, proxy.Snapshot([]byte(tiny), true).Hash, []byte(tiny)).Error)
	for _, source := range []string{"http: [", "http: {routers: []}"} {
		x.put("dynamic/sibling.yml", source)
		_, err := x.attempt(r)
		requireCode(t, err, dto.InvalidGraph)
		var diagnostic *dto.Problem
		require.ErrorAs(t, err, &diagnostic)
		require.Equal(t, "dynamic/sibling.yml", diagnostic.Path)
	}
	other := strings.ReplaceAll(tiny, "app", "other")
	x.put("dynamic/sibling.yml", other)
	require.NoError(t, x.db.Exec("UPDATE proxy_verified_files SET hash=?,content=? WHERE source=? AND path=?", proxy.Snapshot([]byte(other), true).Hash, []byte(other), key, r.Target).Error)
	_, err := x.attempt(r)
	requireCode(t, err, dto.InvalidGraph)
}

func TestPreparedProbeIdentitiesFollowCreateRenameAndTLSDelete(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Target = "dynamic/new.yml"
	r.Change = dto.Change{Kind: "create-route", Name: "new", Rule: "Host(`new.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
	in := x.prepare(r)
	s, err := state(in)
	require.NoError(t, err)
	require.Len(t, s.Probes, 2)
	for _, p := range s.Probes {
		require.Equal(t, "dynamic/new.yml#new", p.Route)
		require.Equal(t, "new.example.com", p.Host)
	}
	r = x.request("edit-route")
	r.Change.Name = "renamed"
	in = x.prepare(r)
	s, err = state(in)
	require.NoError(t, err)
	require.NotEmpty(t, s.Probes)
	for _, p := range s.Probes {
		require.Equal(t, "dynamic/routes.yml#renamed", p.Route)
	}
	x.put("dynamic/routes.yml", strings.Replace(tiny, "      entryPoints: [web]", "      tls: {}\n      entryPoints: [web]", 1))
	in = x.prepare(x.request("delete-route"))
	s, err = state(in)
	require.NoError(t, err)
	require.Len(t, s.Probes, 1)
	require.Equal(t, "https://127.0.0.1:80/", s.Probes[0].URL)
	require.Equal(t, "app.example.com", s.Probes[0].Host)
}

func TestStagedBytesAndResolverSelectionAreValidatedAgain(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	in.New = proxy.Snapshot([]byte(strings.Replace(tiny, "      service: app", "      tls: {certResolver: absent}\n      service: app", 1)), true)
	require.NoError(t, x.d.Stage(t.Context(), in))
	requireCode(t, x.validate(in), dto.InvalidGraph)
	require.NoError(t, os.Remove(filepath.Join(x.root, in.TempPath)))
	in = x.prepare(x.request("edit-route"))
	require.NoError(t, x.d.Stage(t.Context(), in))
	x.put(in.TempPath, tiny+"# unexpected bytes\n")
	requireCode(t, x.validate(in), dto.InvalidGraph)
}

func TestScalarTagsAnchorsTemplatesAndFlowCRLFStayReadOnly(t *testing.T) {
	for _, source := range []string{
		strings.Replace(tiny, "rule: Host", "rule: &rule Host", 1),
		strings.Replace(tiny, "rule: Host", "rule: !!str Host", 1),
		strings.Replace(tiny, "app.example.com", "{{ .Name }}", 1),
		"http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app}}}\r\n",
	} {
		require.NotPanics(t, func() { _, err := parseDocument(source, "routes.yml"); require.Error(t, err) })
	}
}

func TestQuotedScalarEndAndPlainKeyAtEOFHaveProvenSpans(t *testing.T) {
	prefix := "http:\n  middlewares:\n    h:\n      headers:\n        customResponseHeaders:\n          X-Example: "
	for _, scalar := range []string{"'end\\'", "'it''s'", "'end'", `"escaped\\path"`, `"escaped\"quote"`} {
		source := prefix + scalar
		require.NotPanics(t, func() {
			d, err := parseDocument(source, "")
			require.NoError(t, err)
			require.NoError(t, validateDocument(d, ""))
			require.NotNil(t, at(d.root, "http", "middlewares", "h", "headers", "customResponseHeaders", "X-Example"))
		})
	}
	require.NotPanics(t, func() { _, err := parseDocument("http:", ""); require.NoError(t, err) })
}

func TestSpliceBoundsOverlapAndResultSyntaxAreValidated(t *testing.T) {
	source := "# comment is retained\n" + tiny
	for _, edits := range [][]edit{
		{{span{-1, 1}, "x"}},
		{{span{0, len(source) + 1}, "x"}},
		{{span{3, 1}, "x"}},
		{{span{4, 8}, "x"}, {span{2, 6}, "y"}},
		{{span{0, len(source)}, "http: ["}},
	} {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		require.NotPanics(t, func() { _, err = d.apply(edits); require.Error(t, err) })
	}
	d, err := parseDocument(tiny, "")
	require.NoError(t, err)
	for _, n := range []*yaml.Node{nil, at(d.root, "http", "routers", "app", "rule")} {
		require.NotPanics(t, func() { var edits []edit; require.Error(t, d.patch(n, map[string]any{"priority": 10}, &edits)) })
	}
}

func TestFlowSequencesPreservePerItemQuotesAndComments(t *testing.T) {
	source := strings.Replace(tiny, "servers: [{url: 'http://192.0.2.1:8080'}]", "servers: [\n          {url: 'http://192.0.2.1:8080'}, # Endpoint\n        ]", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "services", "app", "loadBalancer"), map[string]any{"servers": servers([]string{"http://192.0.2.2:8080"})}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "192.0.2.1", "192.0.2.2", 1), out)
	source = strings.Replace(tiny, "entryPoints: [web]", "entryPoints: ['web', \"websecure\"]", 1)
	d, err = parseDocument(source, "")
	require.NoError(t, err)
	edits = nil
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"entryPoints": []string{"other", "websecure"}}, &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "['web'", "['other'", 1), out)
	for _, from := range []string{"[{url: 'http://192.0.2.1:8080'}, {url: 'http://192.0.2.3:8080'}]", "[]"} {
		source = strings.Replace(tiny, "[{url: 'http://192.0.2.1:8080'}]", from, 1)
		d, err = parseDocument(source, "")
		require.NoError(t, err)
		edits = nil
		require.NotPanics(t, func() {
			require.NoError(t, d.patch(at(d.root, "http", "services", "app", "loadBalancer"), map[string]any{"servers": servers([]string{"http://192.0.2.2:8080"})}, &edits))
		})
		out, err = d.apply(edits)
		require.NoError(t, err)
		next, err := parseDocument(out, "")
		require.NoError(t, err)
		values := at(next.root, "http", "services", "app", "loadBalancer", "servers")
		require.Len(t, values.Content, 1)
		require.Equal(t, "http://192.0.2.2:8080", str(field(values.Content[0], "url")))
	}
	d, err = parseDocument(strings.Replace(tiny, "entryPoints: [web]", "entryPoints: []", 1), "")
	require.NoError(t, err)
	edits = nil
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"entryPoints": []string{"web"}}, &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	next, err := parseDocument(out, "")
	require.NoError(t, err)
	require.Equal(t, []string{"web"}, list(at(next.root, "http", "routers", "app", "entryPoints")))
}

func TestNoOpSplicesRetainNoncanonicalScalarSpelling(t *testing.T) {
	source := strings.Replace(tiny, "rule: Host(`app.example.com`)", `rule: "Host(\u0060app.example.com\u0060)"`+"\n      priority: 010", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"rule": "Host(`app.example.com`)", "priority": 8}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, source, out)
}

func TestMapReplacementRemovalAndEOFInsertionsKeepSourceContracts(t *testing.T) {
	source := strings.Replace(tiny, "      service: app", "      tls: {certResolver: primary}\n      service: app", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"tls": map[string]any{}}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, out, "tls: {}")
	d, err = parseDocument(source, "")
	require.NoError(t, err)
	edits = nil
	require.Error(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"tls": map[string]any{"certResolver": nil}}, &edits))
	d, err = parseDocument(source, "")
	require.NoError(t, err)
	edits = nil
	require.Error(t, d.patch(at(d.root, "http", "routers", "app", "tls"), map[string]any{"certResolver": nil}, &edits))
	d, err = parseDocument(strings.Replace(source, "{certResolver: primary}", "{}", 1), "")
	require.NoError(t, err)
	edits = nil
	require.NotPanics(t, func() {
		require.NoError(t, d.patch(at(d.root, "http", "routers", "app", "tls"), map[string]any{"certResolver": "primary"}, &edits))
	})
	out, err = d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, out, "tls: {certResolver: primary}")
	source = "http:\n  services: {app: {loadBalancer: {servers: [{url: 'http://192.0.2.1:8080'}]}}}\n  routers:\n    app:\n      rule: Host(`app.example.com`)\n      service: app"
	d, err = parseDocument(source, "")
	require.NoError(t, err)
	edits = nil
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"priority": 10}, &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, source+"\n      priority: 10\n", out)
}

func TestRemovalRetainsOnlyRealCommentsAndHandlesQuotedEOF(t *testing.T) {
	prefix := "http:\n  middlewares:\n    h:\n      headers:\n        customResponseHeaders:\n          Keep: value\n"
	for _, value := range []string{"'it''s # literal' # Retained", `"a # literal" # Retained`, "'end\\' # Retained", "'end'"} {
		source := prefix + "          Remove: " + value
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		var edits []edit
		require.NotPanics(t, func() {
			require.NoError(t, d.patch(at(d.root, "http", "middlewares", "h", "headers", "customResponseHeaders"), map[string]any{"Remove": nil}, &edits))
		})
		out, err := d.apply(edits)
		require.NoError(t, err)
		expected := prefix
		if strings.Contains(value, "Retained") {
			expected += "          # Retained\n"
		}
		require.Equal(t, expected, out)
	}
}

func TestCollidingInsertionsAndEarlierScalarSpliceHaveStableOrder(t *testing.T) {
	source := strings.Replace(tiny, "      entryPoints: [web]\n      service: app\n  services:\n    app:\n      loadBalancer:\n        servers: [{url: 'http://192.0.2.1:8080'}]\n", "      service: app\n      middlewares:\n", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	pos := len(source)
	edits := []edit{{span{pos, pos}, "      priority: 10\n"}, {span{pos, pos}, "        - auth\n"}, {d.spans[at(d.root, "http", "routers", "app", "rule")], "Host(`new.example.com`)"}}
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "app.example.com", "new.example.com", 1)+"        - auth\n      priority: 10\n", out)
}

func TestCreateTargetRefusalsKeepTheirTypedBoundary(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Change = dto.Change{Kind: "create-route", Name: "new", Rule: "Host(`new.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
	for _, target := range []string{"dynamic//new.yml", "dynamic/new.secret.yml", "dynamic/new.toml"} {
		r.Target = target
		_, err := x.attempt(r)
		requireCode(t, err, dto.InvalidInput)
	}
}

func TestRestorePreservesAStoreFailureAfterReadingTheGraph(t *testing.T) {
	x := fixture(t)
	r := x.request("restore-owned")
	var queries atomic.Int64
	require.NoError(t, x.db.Callback().Query().Before("gorm:query").Register("example_restore_store_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "proxy_verified_files" && queries.Add(1) == 2 {
			require.NoError(t, x.db.Exec("DROP TABLE proxy_verified_files").Error)
		}
	}))
	_, err := x.attempt(r)
	require.ErrorContains(t, err, "no such table")
}

func TestMarkerLoadAllowsKnownReferencesAndAnsweredAcknowledgedReferences(t *testing.T) {
	for _, acknowledged := range []bool{false, true} {
		x := fixture(t)
		r := x.request("edit-route")
		if acknowledged {
			r.Change.Middlewares = []string{"missing@file"}
			r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
		}
		in := x.prepare(r)
		require.NoError(t, x.e.Mark(t.Context(), in))
		x.probeFunc = func(args []string) hostio.ExecResult {
			if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
				return response(502, "closed")
			}
			if acknowledged {
				return response(200, "old")
			}
			return response(404, "404 page not found\n")
		}
		got, err := x.e.Observe(t.Context(), in, true)
		require.NoError(t, err)
		require.Equal(t, dto.Verified, got.State)
		require.Equal(t, dto.Load, got.Evidence)
	}
}

func TestMarkerProbeRetainsItsCapabilityFailure(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	require.NoError(t, x.e.Mark(t.Context(), in))
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
			return hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}
		}
		return response(200, "old")
	}
	got, err := x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, got.State)
	require.Equal(t, dto.ProbeUnavailable, got.Reason)
}

func TestFlowRouteRemovalAndCorruptRestoreKeepTypedDiagnostics(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", "http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app}}, services: {app: {loadBalancer: {servers: [{url: 'http://192.0.2.1:8080'}]}}}}\n")
	_, err := x.attempt(x.request("delete-route"))
	requireCode(t, err, dto.UnsupportedFormat)
	x.put("dynamic/routes.yml", tiny)
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", "host\x00"+x.i.Source.Canonical, "dynamic/routes.yml", "recorded", []byte("http: [")).Error)
	_, err = x.attempt(x.request("restore-owned"))
	requireCode(t, err, dto.InvalidGraph)
}

func TestRenameWithSharedServiceRetainsEveryDependantIdentity(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", strings.Replace(tiny, "  routers:\n", "  routers:\n    second:\n      rule: Host(`second.example.com`)\n      service: app\n      entryPoints: [web]\n      middlewares: [existing@file]\n", 1))
	r := x.request("edit-route")
	r.Change.Name = "renamed"
	r.Change.Upstreams = []string{"http://192.0.2.2:8080"}
	in := x.prepare(r)
	s, err := state(in)
	require.NoError(t, err)
	ids := []string{}
	for _, p := range s.Probes {
		ids = append(ids, p.Route)
	}
	require.ElementsMatch(t, []string{"dynamic/routes.yml#renamed", "dynamic/routes.yml#second"}, ids)
	x.put("dynamic/routes.yml", strings.Replace(tiny, "  routers:\n", "  routers:\n    \"\":\n      rule: Host(`empty.example.com`)\n      service: app\n", 1))
	r = x.request("edit-route")
	r.Change.Priority = new(10)
	in = x.prepare(r)
	s, err = state(in)
	require.NoError(t, err)
	require.Len(t, s.Probes, 1)
	require.Equal(t, "dynamic/routes.yml#app", s.Probes[0].Route)
}

func TestStructuredServiceAndListenerChangesReachTheCandidate(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", tiny+"    other:\n      loadBalancer:\n        servers: [{url: 'http://192.0.2.2:8080'}]\n")
	r := x.request("edit-route")
	r.Change.Service = "other"
	r.Change.EntryPoints = []string{"websecure"}
	in := x.prepare(r)
	d, err := parseDocument(string(in.New.Content), "")
	require.NoError(t, err)
	require.Equal(t, "other", str(at(d.root, "http", "routers", "app", "service")))
	require.Equal(t, []string{"websecure"}, list(at(d.root, "http", "routers", "app", "entryPoints")))
}

func TestBeforeProbeCapabilityIsPreservedAfterLaterRecovery(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	x.probeFunc = func(args []string) hostio.ExecResult {
		if !strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
			return hostio.ExecResult{ExitCode: 1, Stderr: []byte("wget: invalid option")}
		}
		return response(200, "old")
	}
	in := x.prepare(r)
	require.NoError(t, x.e.Mark(t.Context(), in))
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
			return response(404, "404 page not found\n")
		}
		return response(200, "recovered")
	}
	got, err := x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, got.State)
	require.Equal(t, dto.ProbeOptionUnavailable, got.Reason)
}

func TestReferenceProviderSuffixAndDuplicateNamespacesStayDistinct(t *testing.T) {
	d, err := parseDocument(tiny+"    \"\": {loadBalancer: {servers: [{url: 'http://192.0.2.2:8080'}]}}\n", "")
	require.NoError(t, err)
	docs := map[string]*document{"routes.yml": d}
	require.Equal(t, "unresolved", resolve(docs, nil, dto.Inventory{Providers: []string{"docker"}}, "services", "docker").Kind)
	require.Equal(t, "unresolved", resolve(docs, nil, dto.Inventory{}, "services", "foreign@absent").Kind)
	other, err := parseDocument(tiny, "")
	require.NoError(t, err)
	docs["other.yml"] = other
	require.Equal(t, "unresolved", resolve(docs, []dto.OpaqueFile{{Path: "auth.secret.yml"}}, dto.Inventory{}, "services", "app").Kind)
}

func TestCrossFileServiceAndExternalDependantsRemainTextOnly(t *testing.T) {
	x := fixture(t)
	split := strings.Index(tiny, "  services:")
	x.put("dynamic/routes.yml", tiny[:split])
	x.put("dynamic/services.yml", "http:\n"+tiny[split:])
	read, err := x.readResult()
	require.NoError(t, err)
	require.Equal(t, "text-only", read.Routes[0].Class)
	require.Equal(t, dto.SharedAcrossFiles, read.Routes[0].Eligibility.Reason)
	d, err := parseDocument(strings.Replace(tiny, "service: app", "service: api@internal", 1), "")
	require.NoError(t, err)
	routes := classify(map[string]*document{"routes.yml": d}, nil, dto.Inventory{})
	require.Empty(t, routes[0].Dependants)
}

func TestNodeReadersDoNotCoerceIncompatibleShapes(t *testing.T) {
	d, err := parseDocument("number: 123\npair: [rule, value]\nmap: {rule: value}\n", "")
	require.NoError(t, err)
	require.Equal(t, "", str(field(d.root, "number")))
	require.Empty(t, list(field(d.root, "map")))
	require.Empty(t, keys(field(d.root, "pair")))
	require.Nil(t, field(field(d.root, "pair"), "rule"))
	require.NotPanics(t, func() { require.Nil(t, value(field(d.root, "missing"))) })
}

func TestGenericSpanReplacementPreservesTheRequestedShape(t *testing.T) {
	for _, tt := range []struct {
		source      string
		replacement any
		tag         string
	}{
		{"field: old\n", map[string]any{"key": "value"}, "!!map"},
		{"field:\n  old: value\n", []any{"first", "second"}, "!!seq"},
		{"field:\n  old: value\n", []string{"first", "second"}, "!!seq"},
		{"field:\n  old: value\n", []string{"first"}, "!!seq"},
		{"field:\n  - first\n  - second\n", []any{}, "!!seq"},
	} {
		d, err := parseDocument(tt.source, "")
		require.NoError(t, err)
		var edits []edit
		require.NotPanics(t, func() { require.NoError(t, d.patch(d.root, map[string]any{"field": tt.replacement}, &edits)) })
		out, err := d.apply(edits)
		require.NoError(t, err)
		next, err := parseDocument(out, "")
		require.NoError(t, err)
		require.Equal(t, tt.tag, field(next.root, "field").Tag)
		expected := tt.replacement
		if strings, ok := expected.([]string); ok {
			items := []any{}
			for _, value := range strings {
				items = append(items, value)
			}
			expected = items
		}
		require.Equal(t, expected, value(field(next.root, "field")))
	}
}
