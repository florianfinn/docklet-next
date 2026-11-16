// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"encoding/json/v2"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInventoryStaticSelectorsAndFallbackFaults(t *testing.T) {
	x := fixture(t)
	static := func(port string) string {
		return "providers: {file: {directory: /etc/traefik/dynamic}}\nentryPoints: {web: {address: ':" + port + "'}}\n"
	}
	x.put("chosen.yml", static("80"))
	x.put("other.yml", static("8080"))
	cases := []struct {
		args, env []string
		root      string
		writable  bool
		port      string
	}{
		{[]string{"--configFile=/etc/traefik/chosen.yml"}, nil, "", true, ":80"},
		{nil, []string{"TRAEFIK_CONFIGFILE=/etc/traefik/chosen.yml"}, "", true, ":80"},
		{[]string{"--configFile=/etc/traefik/chosen.yml"}, []string{"TRAEFIK_CONFIGFILE=/etc/traefik/other.yml"}, "", true, ":80"},
		{[]string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:80"}, nil, "/etc/traefik/missing.yml", false, ""},
		{nil, nil, "relative.yml", false, ""},
		{nil, nil, "/etc/traefik/../chosen.yml", false, ""},
		{nil, nil, "/etc/traefik/hidden.secret.yml", false, ""},
	}
	for _, tt := range cases {
		x.inspect.Config.Cmd = tt.args
		x.inspect.Config.Env = tt.env
		x.i.RootFile = tt.root
		v, err := x.inventory()
		require.NoError(t, err)
		require.Equal(t, tt.writable, v.Eligibility.Writable, tt)
		if tt.port != "" {
			require.Equal(t, tt.port, v.EntryPoints[0].Address)
		}
	}
	x.i.RootFile = ""
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:80"}
	x.inspect.Config.Env = nil
	x.archiveStatus = map[string]int{"/etc/traefik/traefik.yml": 403}
	v, err := x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
	x.archiveStatus = nil
	x.inspect.Mounts[0].RW = false
	x.i.Source.ReadOnly = true
	v, err = x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
}
func TestInventoryArgumentBoundariesAndIdentityProjection(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Entrypoint = []string{"traefik"}
	x.inspect.Config.Cmd = nil
	x.inspect.Config.Env = []string{"TRAEFIK_PROVIDERS_FILE_DIRECTORY=/etc/traefik/dynamic", "TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=:80", "ENTRYPOINTS_WEB_ADDRESS=:666", "TRAEFIK_ENTRYPOINTS_BAD_ADDRESS"}
	v, err := x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
	require.Len(t, v.EntryPoints, 1)
	require.Equal(t, ":80", v.EntryPoints[0].Address)
	x.inspect.Config.Env = nil
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik/dynamic", "--providers.docker", "--entrypoints.web.address=:80"}
	v, err = x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
	require.Len(t, v.EntryPoints, 1)
	require.Equal(t, "web", v.EntryPoints[0].Name)
	x.inspect.State.StartedAt = epoch.Add(time.Minute).Format(time.RFC3339Nano)
	v, err = x.inventory()
	require.NoError(t, err)
	require.Equal(t, x.inspect.State.StartedAt, v.Identity.StartedAt)
}
func TestLabelLookupFailureIsReported(t *testing.T) {
	x := fixture(t)
	require.NoError(t, x.db.Exec("DROP TABLE projects").Error)
	_, err := x.e.Read(t.Context(), x.i)
	require.Error(t, err)
}

func TestInventoryMixedSourcesAndProviderAbsence(t *testing.T) {
	x := fixture(t)
	static := "providers: {file: {directory: /etc/traefik/dynamic}}\nentryPoints: {web: {address: ':80'}}\n"
	x.put("chosen.yml", static)
	x.inspect.Config.Cmd = []string{"--configFile=/etc/traefik/chosen.yml", "--log.level=INFO"}
	v, err := x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
	x.inspect.Config.Cmd = []string{"--configFile=/etc/traefik/chosen.yml"}
	x.inspect.Config.Env = []string{"TRAEFIK_LOG_LEVEL=INFO"}
	v, err = x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:80"}
	x.inspect.Config.Env = []string{"TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=:80"}
	v, err = x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
	x.inspect.Config.Env = nil
	v, err = x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
	require.Equal(t, []string{"file"}, v.Providers)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--providers.docker=false")
	v, err = x.inventory()
	require.NoError(t, err)
	require.Equal(t, []string{"file"}, v.Providers)
	x.inspect.Config.Cmd = []string{"--configFile=/etc/traefik/chosen.yml"}
	x.put("chosen.yml", "providers: [")
	v, err = x.inventory()
	require.NoError(t, err)
	require.False(t, v.Eligibility.Writable)
}
func TestProbeCapabilitiesAndMissingLogEpoch(t *testing.T) {
	for _, tt := range []struct {
		res    hostio.ExecResult
		reason dto.Reason
	}{{hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}, dto.ProbeUnavailable}, {hostio.ExecResult{ExitCode: 1, Stderr: []byte("wget: unrecognized option")}, dto.ProbeOptionUnavailable}} {
		x := fixture(t)
		x.probeFunc = func([]string) hostio.ExecResult { return tt.res }
		in := x.prepare(x.request("edit-route"))
		obs, err := x.e.Observe(t.Context(), in, false)
		require.NoError(t, err)
		require.Equal(t, dto.AppliedUnverified, obs.State)
		require.Equal(t, tt.reason, obs.Reason)
	}
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	in.PublishedAt = time.Time{}
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.LogsUnavailable, obs.Reason)
}

func TestInventoryProviderDirectoryAndPorts(t *testing.T) {
	x := fixture(t)
	base := []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:80"}
	for _, tt := range []struct {
		args     []string
		writable bool
		code     dto.Reason
	}{
		{[]string{"--entrypoints.web.address=:80"}, false, dto.IncompleteInventory},
		{[]string{"--providers.file.filename=/etc/traefik/dynamic/routes.yml", "--entrypoints.web.address=:80"}, false, dto.SingleFileMount},
		{[]string{"--providers.file.directory=relative", "--entrypoints.web.address=:80"}, false, dto.IncompleteInventory},
		{[]string{"--providers.file.directory=/etc/traefik/../dynamic", "--entrypoints.web.address=:80"}, false, dto.IncompleteInventory},
		{[]string{"--providers.file.directory=/etc/traefik/other", "--entrypoints.web.address=:80"}, false, dto.IncompleteInventory},
		{[]string{"--providers.file.directory=/etc/traefik/dynamic"}, false, dto.IncompleteInventory},
		{append(append([]string{}, base...), "--entrypoints.web.address=:letters"), false, dto.IncompleteInventory},
	} {
		x.inspect.Config.Cmd = tt.args
		v, err := x.inventory()
		require.NoError(t, err)
		require.Equal(t, tt.writable, v.Eligibility.Writable)
		require.Equal(t, tt.code, v.Eligibility.Reason)
	}
	x.inspect.Config.Cmd = base
	x.i.RouteDirectory = "/etc/traefik/dynamic"
	v, err := x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
}

func TestUnknownReferenceNoRouteResponseRestoresWithoutLogError(t *testing.T) {
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
			return response(404, "404 page not found\n")
		}
		return response(200, "old")
	}
	op, err := a.Apply(t.Context(), r)
	require.NoError(t, err)
	require.Equal(t, dto.Restored, op.State)
	require.Equal(t, dto.ProxyRejected, op.Reason)
	require.Equal(t, tiny, x.read(r.Target))
}
func TestResolverChangeAndEmptyProbeSetRequireMarker(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.primary.acme.email=operator@example.com")
	r.Change.Resolver = "primary"
	in := x.prepare(r)
	x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.NeedsMarker)
	s, err := state(in)
	require.NoError(t, err)
	s.Probes = nil
	s.ForceMarker = false
	in.Engine, err = json.Marshal(s)
	require.NoError(t, err)
	obs, err = x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.NeedsMarker)
}
func TestProbeFailureAfterPrepareKeepsCapabilityReason(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik/dynamic", "--entrypoints.web.address=:443", "--entrypoints.web.http.tls=true"}
	in := x.prepare(x.request("edit-route"))
	x.probeFunc = func([]string) hostio.ExecResult {
		return hostio.ExecResult{ExitCode: 127, Stderr: []byte("wget: not found")}
	}
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.Equal(t, dto.ProbeUnavailable, obs.Reason)
}
func TestUnknownReferenceLosingRouteDuringMarkerRestores(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	r.Change.Middlewares = []string{"missing@file"}
	r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
	in := x.prepare(r)
	obs, err := x.e.Observe(t.Context(), in, false)
	require.NoError(t, err)
	require.True(t, obs.NeedsMarker)
	require.NoError(t, x.e.Mark(t.Context(), in))
	x.probeFunc = func(args []string) hostio.ExecResult {
		if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
			return response(502, "closed")
		}
		return response(404, "404 page not found\n")
	}
	obs, err = x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.True(t, obs.Restore)
}

func TestCreateRefusesUnconsumedAndOutOfProviderFiles(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Expected = dto.FileState{}
	r.Change = dto.Change{Kind: "create-route", Name: "new-route", Rule: "Host(`new.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
	for _, target := range []string{"dynamic/new.txt", "outside/new.yml"} {
		r.Target = target
		_, err := x.e.Prepare(t.Context(), r)
		requireCode(t, err, dto.InvalidInput)
	}
	r = x.request("edit-route")
	r.Change.Service = "other"
	r.Change.Upstreams = []string{"http://192.0.2.2:8080"}
	_, err := x.e.Prepare(t.Context(), r)
	requireCode(t, err, dto.InvalidInput)
}

func TestRuntimeReadFailuresArePreserved(t *testing.T) {
	x := fixture(t)
	r := x.request("edit-route")
	in := x.prepare(r)
	require.NoError(t, x.d.Stage(t.Context(), in))
	x.inspectFailures = 1
	_, err := x.readResult()
	require.ErrorContains(t, err, "container unavailable")
	x.inspectFailures = 1
	_, err = x.e.Prepare(t.Context(), r)
	require.ErrorContains(t, err, "container unavailable")
	x.inspectFailures = 1
	err = x.e.Validate(t.Context(), in)
	require.ErrorContains(t, err, "container unavailable")
	x.archiveStatus = map[string]int{"/etc/traefik/dynamic": 503}
	_, err = x.readResult()
	require.ErrorContains(t, err, "archive unavailable")
	x.archiveStatus = nil
	x.archiveStatus = map[string]int{"/etc/traefik/" + in.TempPath: 503}
	err = x.e.Validate(t.Context(), in)
	require.ErrorContains(t, err, "archive unavailable")
	x.archiveStatus = nil
	require.NoError(t, x.db.Exec("DROP TABLE proxy_verified_files").Error)
	_, err = x.readResult()
	require.Error(t, err)
}
func TestOpaqueGraphHashesReachTheJournal(t *testing.T) {
	x := fixture(t)
	x.now = time.Now()
	x.put("dynamic/extra.secret.yml", "opaque")
	a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	r := x.request("edit-route")
	r.Generation = a.Generation()
	r.Instance.Generation = r.Generation
	r.Instance.Registration = r.Registration
	r.Change.Priority = new(10)
	x.probeFunc = func([]string) hostio.ExecResult {
		b, err := os.ReadFile(filepath.Join(x.root, r.Target))
		if err == nil && strings.Contains(string(b), "priority:") {
			return response(200, "changed")
		}
		return response(200, "old")
	}
	op, err := a.Apply(t.Context(), r)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, op.State)
	require.Equal(t, dto.Routing, op.Evidence)
}
func TestMarkerWriterFailureDoesNotPanic(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	x.inspect.Mounts[0].Source = filepath.Join(x.root, "replaced")
	require.NotPanics(t, func() { require.Error(t, x.e.Mark(t.Context(), in)) })
}

func TestMarkerAtProviderRootAndLongThrottle(t *testing.T) {
	x := fixture(t)
	require.NoError(t, os.Rename(filepath.Join(x.root, "dynamic", "routes.yml"), filepath.Join(x.root, "routes.yml")))
	x.i.RouteDirectory = "."
	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik", "--entrypoints.web.address=:80", "--providers.providersThrottleDuration=2m"}
	v, err := x.inventory()
	require.NoError(t, err)
	require.True(t, v.Eligibility.Writable)
	require.Equal(t, int64(120000), v.ThrottleMillis)
	r := x.request("edit-route")
	r.Target = "routes.yml"
	r.Expected = proxy.Snapshot([]byte(tiny), true).FileState
	r.Change.RouteID = "routes.yml#app"
	in := x.prepare(r)
	in.MarkerPath = "docklet-check-example-op.yml"
	in.TempPath = ".docklet-example-op.tmp"
	require.NoError(t, x.e.Mark(t.Context(), in))
	require.Contains(t, x.read(in.MarkerPath), "Host(`example-op.docklet-check.invalid`)")
	require.NoError(t, x.d.Cleanup(t.Context(), in))
}

func TestSingleFileProviderRemainsReadableAndReadOnly(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = []string{"--providers.file.filename=/etc/traefik/dynamic/routes.yml", "--entrypoints.web.address=:80"}
	x.i.Source.Canonical = filepath.Join(x.root, "dynamic", "routes.yml")
	x.i.Source.MountTarget = "/etc/traefik/dynamic/routes.yml"
	x.i.Source.Kind = dto.SingleFileSource
	x.i.Source.ReadOnly = true
	x.i.Source.Reason = dto.SingleFileMount
	x.i.RouteDirectory = "."
	x.inspect.Mounts[0].Source = x.i.Source.Canonical
	x.inspect.Mounts[0].Destination = x.i.Source.MountTarget
	r, err := x.readResult()
	require.NoError(t, err)
	require.False(t, r.Eligibility.Writable)
	require.Equal(t, dto.SingleFileMount, r.Eligibility.Reason)
	require.Len(t, r.Files, 1)
	require.Equal(t, tiny, r.Files[0].Content)
	require.Len(t, r.Routes, 1)
	require.Equal(t, "app", r.Routes[0].Name)
	require.Equal(t, tiny, x.read("dynamic/routes.yml"))
}

func TestFilenameProviderOnDirectorySourceReadsOnlyItsFile(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = []string{"--providers.file.filename=/etc/traefik/dynamic/routes.yml", "--entrypoints.web.address=:80"}
	x.put("dynamic/ignored.yml", "http: [")
	x.i.Source.ReadOnly = true
	x.i.Source.Reason = dto.ReadOnlyMount
	x.inspect.Mounts[0].RW = false
	r, err := x.readResult()
	require.NoError(t, err)
	require.Len(t, r.Files, 1)
	require.Equal(t, "dynamic/routes.yml", r.Files[0].Path)
	require.Equal(t, dto.ReadOnlyMount, r.Eligibility.Reason)
}

func TestOpaqueFilenameProviderProjectsOnlyRelativeName(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/auth.secret.yml", "do-not-project-this-payload")
	x.inspect.Config.Cmd = []string{"--providers.file.filename=/etc/traefik/dynamic/auth.secret.yml", "--entrypoints.web.address=:80"}
	r, err := x.readResult()
	require.NoError(t, err)
	require.Empty(t, r.Files)
	require.Equal(t, []dto.OpaqueFile{{Path: "dynamic/auth.secret.yml"}}, r.OpaqueFiles)
	for _, p := range x.archives {
		require.False(t, hostio.Secret(p))
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	require.NotContains(t, string(b), "do-not-project-this-payload")
}
