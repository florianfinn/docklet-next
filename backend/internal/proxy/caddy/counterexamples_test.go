// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original isolated-operand counterexamples at the Docker seam.

package caddy

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func TestOutsideSelectedSourceCannotEnableImportInsertion(t *testing.T) {
	x := fixture(t)
	x.i.RouteDirectory = "/outside/routes"
	x.put("outside/routes/example.caddy", "outside.example.net {\n respond ok\n}\n")
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "outside/routes"), Destination: "/outside/routes", RW: true})
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.SourceReadOnly, inv.Eligibility.Reason)
}
func TestMissingForeignImportDirectoryIsNotAMissingRouteDirectory(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", rootText+"import /etc/caddy/missing/*.caddy\n")
	_, err := readChecked(t, x.e, x.i)
	require.True(t, errdefs.IsNotFound(err))
}
func TestRootImportIgnoresUnrelatedReadOnlyChildMount(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n"))
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "unrelated"), Destination: "/etc/caddy/unrelated", RW: false})
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
}
func TestUnknownSourceDirectoryHasNoEligibleRouteMount(t *testing.T) {
	x := fixture(t)
	x.c.Mounts[0].Type = mount.TypeTmpfs
	x.c.Mounts[0].Source = ""
	x.i.Source.Kind = dto.UnknownSourceKind
	x.i.Source.ReadOnly = true
	x.i.Source.Canonical = "."
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.ImportMissing, inv.Eligibility.Reason)
}
func TestSourceNegotiationFailureAfterInspectionIsPreserved(t *testing.T) {
	x := fixture(t)
	inspections := 0
	closed := false
	failed := 0
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			inspections++
			if inspections == 2 {
				x.d.Docker.Close()
				closed = true
			}
		}
		if closed && failed < 2 && strings.HasSuffix(r.URL.Path, "/_ping") {
			failed++
			http.Error(w, `{"message":"negotiation unavailable"}`, 500)
			return true
		}
		return false
	}
	_, err := inventoryChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "failed to create Docker client")
	require.Equal(t, 2, failed)
}

func TestImportedTextFileInRouteDirectoryIsNotAnOwnedRoute(t *testing.T) {
	x := fixture(t)
	text, err := renderChecked(t, baseSite())
	require.NoError(t, err)
	x.put("routes/extra.txt", text)
	x.put("Caddyfile", rootText+"import /etc/caddy/routes/extra.txt\n")
	r, err := readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 1)
	require.Len(t, r.Files, 2)
}
func TestNestedRootHandlersAreNotAdditionalSites(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", "legacy.example.net {\n route {\n respond ok\n }\n}\nimport /etc/caddy/routes/*.caddy\n")
	r, err := readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 1)
	require.Equal(t, "legacy.example.net", r.Routes[0].Name)
}
func TestNoncanonicalTargetCannotNormalizeIntoRouteDirectory(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	q.Target = "routes/../routes/app.caddy"
	_, err := prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
}
func TestAdminOffInventoryDoesNotReadTheDisabledEndpoint(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
	x.execFn = func([]string) hostio.ExecResult { return hostio.ExecResult{ExitCode: 1} }
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.AdminOff)
	require.Empty(t, x.commands)
}
func TestProbeVectorsUseTheSiteSchemeAndExactPath(t *testing.T) {
	x := fixture(t)
	for _, tls := range []string{"off", "auto"} {
		for _, p := range []string{"", "/api"} {
			s := baseSite()
			s.TLS = tls
			s.Path = p
			_, err := x.e.probe(t.Context(), x.i, s)
			require.NoError(t, err)
			scheme := "http"
			if tls != "off" {
				scheme = "https"
			}
			urlPath := p
			if urlPath == "" {
				urlPath = "/"
			}
			expected, _, err := hostio.Args(dto.Caddy, "probe", "", s.Domain, scheme+"://127.0.0.1"+urlPath)
			require.NoError(t, err)
			require.Equal(t, expected, x.commands[len(x.commands)-1])
		}
	}
}
func TestStopDuringObservationInvalidatesEvidenceWithoutChangingStartedAt(t *testing.T) {
	x := fixture(t)
	i := observationIntent(x, baseSite(), true, "edit-route")
	x.execFn = func([]string) hostio.ExecResult { x.c.State.Running = false; return httpResult(201, "changed") }
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.ContainerChanged, o.Reason)
}
func TestDeletionChecksTargetAbsenceWithoutReplacingUnrelatedAdminState(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	other := s
	other.Domain = "other.example.net"
	b, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", Admin: "localhost:2019", OldSite: &s, Before: hostio.Response{Status: 200, Headers: map[string]string{"server": "Caddy"}}})
	i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: "delete-route"}}, Engine: b}
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "caddy" {
			return hostio.ExecResult{Stdout: []byte("{}")}
		}
		if strings.Contains(args[len(args)-1], "/config/") {
			return httpResult(200, configFor(other))
		}
		return httpResult(404, "")
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
}
func TestLateRoutingCannotEraseAnEarlierAdminFailure(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	i := observationIntent(x, s, false, "edit-route")
	readbacks := 0
	probes := 0
	x.execFn = func(args []string) hostio.ExecResult {
		if strings.Contains(args[len(args)-1], "/config/") {
			readbacks++
			if readbacks == 1 {
				return hostio.ExecResult{ExitCode: 1}
			}
			return httpResult(200, configFor(s))
		}
		probes++
		if probes == 2 {
			x.now = x.now.Add(4 * time.Second)
			return httpResult(201, "late")
		}
		return httpResult(200, "")
	}
	x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(2500 * time.Millisecond); return nil }
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.AdminUnavailable, o.Reason)
}

func TestP7Distinct404FirstResponseFingerprintsAreRoutingEvidence(t *testing.T) {
	for _, kind := range []string{"create-route", "delete-route"} {
		t.Run(kind, func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			beforeResult := httpResult(404, "old response")
			afterResult := httpResult(404, "new response")
			before, err := hostio.ParseProbe(beforeResult, nil)
			require.NoError(t, err)
			state := state{Root: "/etc/caddy/Caddyfile", AdminOff: true, Before: before, NewSite: &s}
			if kind == "delete-route" {
				state.NewSite = nil
				state.OldSite = &s
			}
			b, _ := json.Marshal(state)
			i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: kind}}, Engine: b}
			x.execFn = func([]string) hostio.ExecResult { return afterResult }
			x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.Verified, o.State)
			require.Equal(t, dto.Routing, o.Evidence)
		})
	}
}

func TestImportedGlobalOptionsAndDynamicModeFailClosed(t *testing.T) {
	x := fixture(t)
	x.put("options.caddy", "{\n admin off\n persist_config off\n}")
	x.put("Caddyfile", "import /etc/caddy/options.caddy\nlegacy.example.net {\n respond ok\n}\nimport /etc/caddy/routes/*.caddy\n")
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.AdminOff)
	require.False(t, inv.PersistConfig)
	require.True(t, inv.Eligibility.Writable)
	q := x.request("create-route", baseSite())
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "acknowledgement-required")
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.PersistenceOff, inv.Eligibility.Reason)
	require.Empty(t, x.commands)
	x.c.Config.Cmd = x.c.Config.Cmd[:len(x.c.Config.Cmd)-1]
	for _, option := range []string{"admin {$ADMIN_MODE}", "persist_config {$PERSIST_MODE}"} {
		x.put("Caddyfile", "{\n"+option+"\n}\nimport /etc/caddy/routes/*.caddy\n")
		inv, err = inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.Eligibility.Writable)
		require.Equal(t, dto.IncompleteInventory, inv.Eligibility.Reason)
	}
	x.put("Caddyfile", "{\n email {$EMAIL}\n}\nimport /etc/caddy/routes/*.caddy\n")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
}
func TestResumeWithOpaqueFilesNeverReadsAdaptedConfiguration(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	x.put("routes/extra.secret.caddy", "opaque payload")
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.OpaqueInput, inv.Eligibility.Reason)
	require.Empty(t, x.commands)
}
func TestGlobalOptionExpansionPreservesScopeAndBounds(t *testing.T) {
	x := fixture(t)
	x.put("handler.caddy", "route {\n respond ok\n}")
	x.put("Caddyfile", "legacy.example.net {\n import /etc/caddy/handler.caddy\n}\nimport /etc/caddy/routes/*.caddy\n")
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.AdminOff)
	require.True(t, inv.Eligibility.Writable)
	x.put("fragment.caddy", "#"+strings.Repeat("a", (1<<20)-2)+"\n")
	text := rootText
	for n := 0; n < 17; n++ {
		text += "import /etc/caddy/fragment.caddy\n"
	}
	x.put("Caddyfile", text)
	_, err = readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "input-limit")
}

func TestExpandedImportDepthIncludesPreviouslyReadDescendants(t *testing.T) {
	x := fixture(t)
	x.put("leaf.caddy", "# leaf\n")
	x.put("shared.caddy", "import /etc/caddy/leaf.caddy\n")
	for n := 0; n < 7; n++ {
		next := "shared.caddy"
		if n < 6 {
			next = fmt.Sprintf("chain-%d.caddy", n+1)
		}
		x.put(fmt.Sprintf("chain-%d.caddy", n), "import /etc/caddy/"+next+"\n")
	}
	x.put("Caddyfile", rootText+"import /etc/caddy/shared.caddy\nimport /etc/caddy/chain-0.caddy\n")
	_, err := readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "input-limit")
}
func TestGlobalOptionsThroughTheRelativeRouteImport(t *testing.T) {
	x := fixture(t)
	x.put("routes/-options.caddy", "{\n admin off\n}\n")
	x.put("Caddyfile", "import routes/*.caddy\nlegacy.example.net {\n respond ok\n}\n")
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.AdminOff)
	require.True(t, inv.Eligibility.Writable)
}

func TestRawGraphBudgetIncludesOrphansAndDeduplicatesImports(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "orphans", true: "deduplication"}[duplicate], func(t *testing.T) {
			x := fixture(t)
			root := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
			size := 16
			if duplicate {
				size = 14
				x.put("shared.caddy", "#"+strings.Repeat("a", hostio.MaxFileBytes-2)+"\n")
				root += "import /etc/caddy/shared.caddy\nimport /etc/caddy/shared.caddy\n"
			}
			x.put("Caddyfile", root)
			for n := 0; n < size; n++ {
				x.put(fmt.Sprintf("routes/orphan-%02d.caddy", n), "#"+strings.Repeat("a", hostio.MaxFileBytes-2)+"\n")
			}
			r, err := readChecked(t, x.e, x.i)
			if duplicate {
				require.NoError(t, err)
				require.True(t, r.Eligibility.Writable)
				require.Equal(t, dto.ImportMissing, r.Eligibility.Reason)
			} else {
				require.ErrorContains(t, err, "input-limit")
			}
		})
	}
}
func TestNonResumeInventoryDoesNotWarnFromAdminDivergence(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "wget" {
			return httpResult(200, configFor(s))
		}
		return hostio.ExecResult{Stdout: []byte("{}")}
	}
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.RunningDiverged)
	require.Empty(t, x.commands)
}
func TestReadbackNestedHostAndPathConstraintsRemainConjunctive(t *testing.T) {
	s := baseSite()
	s.Path = "/api"
	for _, outer := range []string{`"host":["other.example.net"]`, `"host":["app.example.com"],"path":["/other*"]`} {
		raw := `{"apps":{"http":{"servers":{"example":{"routes":[{"match":[{` + outer + `}],"handle":[{"handler":"subroute","routes":[{"match":[{"host":["app.example.com"],"path":["/api*"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"192.0.2.80:8080"}]}]}]}]}]}}}}}`
		var config map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &config))
		require.False(t, containsSite(config, s, true))
	}
}
func TestReadbackNullTLSIsNotAnHTTPSUpstream(t *testing.T) {
	handler := map[string]any{"upstreams": []any{map[string]any{"dial": "example.com:443"}}, "transport": map[string]any{"tls": nil}}
	require.False(t, upstreamsEqual(handler, []string{"https://example.com"}))
	handler["upstreams"] = []any{map[string]any{"dial": "example.com:80"}}
	require.True(t, upstreamsEqual(handler, []string{"http://example.com"}))
}

func TestReadbackNonHTTPTransportCannotProveAnHTTPUpstream(t *testing.T) {
	handler := map[string]any{"upstreams": []any{map[string]any{"dial": "example.com:80"}}, "transport": map[string]any{"protocol": "fastcgi"}}
	require.False(t, upstreamsEqual(handler, []string{"http://example.com"}))
	handler["transport"] = map[string]any{"protocol": "http"}
	require.True(t, upstreamsEqual(handler, []string{"http://example.com"}))
}

func TestDefaultObservationClockWaitHonorsCancellation(t *testing.T) {
	e := New(nil).(*engine)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, e.wait(ctx, time.Hour), context.Canceled)
	require.NoError(t, e.wait(t.Context(), 0))
}

func TestReadbackUnrestrictedSiteDoesNotAcquireAPathMatcher(t *testing.T) {
	s := baseSite()
	raw := strings.Replace(configFor(s), `"host":["app.example.com"]`, `"host":["app.example.com"],"path":["*"]`, 1)
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &config))
	require.False(t, containsSite(config, s, true))
}

func TestCaddyRefusesTraefikAndUnknownOperationKindsBeforeExec(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	for _, kind := range []string{"edit-service", "restore-owned", "unknown"} {
		q.Change.Kind = kind
		_, err := prepareChecked(t, x.e, q)
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		require.Equal(t, dto.InvalidInput, p.Code)
		require.Equal(t, "unsupported change", p.Detail)
	}
	require.Empty(t, x.commands)
}

func TestReadbackRequiresTheRequestedHostAndPathMatchers(t *testing.T) {
	s := baseSite()
	s.Path = "/api"
	raw := configFor(s)
	for _, which := range []string{"host", "path"} {
		var config map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &config))
		servers := object(object(object(config["apps"])["http"])["servers"])
		route := object(array(object(servers["example"])["routes"])[0])
		matcher := object(array(route["match"])[0])
		delete(matcher, which)
		require.False(t, containsSite(config, s, true), which)
	}
}
