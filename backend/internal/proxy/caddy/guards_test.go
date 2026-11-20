// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original inventory, parser and semantic evidence guard corpus.

package caddy

import (
	"context"
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestCommandInventory(t *testing.T) {
	for _, args := range [][]string{{"caddy", "run", "--config", "/etc/caddy/Caddyfile"}, {"caddy", "run", "--config=/etc/caddy/Caddyfile", "--adapter=caddyfile", "--resume=true", "--watch", "--environ"}, {"caddy", "run", "--config=/etc/caddy/Caddyfile", "--resume=false"}} {
		root, resume, err := command(container.InspectResponse{Config: &container.Config{Cmd: args}})
		require.NoError(t, err)
		require.Equal(t, "/etc/caddy/Caddyfile", root)
		require.Equal(t, strings.Contains(strings.Join(args, " "), "--resume=true"), resume)
	}
	// Without --config Caddy reads ./Caddyfile from its working directory; without --adapter
	// it guesses from the file name. Neither is assumed.
	bad := [][]string{{"caddy"}, {"caddy", "run"}, {"caddy", "run", "--resume=false"}, {"caddy", "run", "--config=/etc/caddy/config.json"}, {"caddy", "run", "--config=/etc/caddy/Caddyfile.JSON"}, {"caddy", "run", "--config"}, {"caddy", "run", "--adapter=json"}, {"caddy", "run", "--resume=maybe"}, {"caddy", "run", "--unknown"}, {"caddy", "run", "--config=relative"}, {"caddy", "run", "--config=/etc/../Caddyfile"}, {"caddy", "run", "--config=/etc/Caddyfile\n"}}
	for _, args := range bad {
		_, _, err := command(container.InspectResponse{Config: &container.Config{Cmd: args}})
		require.Error(t, err, "%v", args)
	}
	_, _, err := command(container.InspectResponse{})
	require.Error(t, err)
}
func TestScannerAndImportSpans(t *testing.T) {
	text := "# import ignored\n{\n admin off\n}\nexample.com {\n tls {\n dns cloudflare {env.TOKEN}\n }\n import \"/etc/caddy/routes/*.caddy\" # after\n}\n"
	ds, err := scan(text)
	require.NoError(t, err)
	var imported directive
	for _, d := range ds {
		if d.words[0] == "import" {
			imported = d
		}
	}
	require.Equal(t, 1, imported.depth)
	require.Equal(t, []string{"import", "/etc/caddy/routes/*.caddy"}, imported.words)
	require.Equal(t, " import \"/etc/caddy/routes/*.caddy\" # after\n", text[imported.start:imported.end])
	for _, bad := range []string{"}", "example.com {\n", "import \"unterminated\n", "import \"escaped\\"} {
		_, err := scan(bad)
		require.Error(t, err)
	}
	_, err = scan("example.com {\n respond `a # b`\n header X-Test \"a\\\"b\"\n}\n")
	require.NoError(t, err)
}
func TestAdminURLRefusals(t *testing.T) {
	for _, address := range []string{"localhost:2019", ":2019", "0.0.0.0:2019", "[::1]:2019", "[::]:2019", "http://127.0.0.1:2019/"} {
		u, err := adminURLChecked(t, address)
		require.NoError(t, err)
		require.Equal(t, "http://127.0.0.1:2019/config/", u)
	}
	for _, address := range []string{"http://127.0.0.1", "localhost"} {
		u, err := adminURLChecked(t, address)
		require.NoError(t, err)
		require.Equal(t, "http://127.0.0.1:2019/config/", u)
	}
	for _, address := range []string{"%", "https://localhost:2019", "http://user@localhost:2019", "localhost:2019?query=1", "localhost:2019#fragment", "localhost:2019/path", "example.com:2019", "unix//admin.sock"} {
		_, err := adminURLChecked(t, address)
		require.Error(t, err, address)
	}
}
func TestReadbackRequiresOneMatchingBranch(t *testing.T) {
	s := baseSite()
	s.Path = "/api"
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(configFor(s)), &config))
	require.True(t, containsSite(config, s, true))
	for _, change := range []func(*Site){func(s *Site) { s.Domain = "other.example.net" }, func(s *Site) { s.Path = "/other" }, func(s *Site) { s.Path = "" }, func(s *Site) { s.Upstreams = []string{"http://192.0.2.90:8080"} }} {
		candidate := s
		change(&candidate)
		require.False(t, containsSite(config, candidate, true))
	}
	require.True(t, containsSite(config, s, false))
	require.False(t, upstreamsEqual(map[string]any{"upstreams": []any{map[string]any{"dial": "192.0.2.80:8080"}}}, []string{"https://192.0.2.80:8080"}))
	require.False(t, upstreamsEqual(map[string]any{}, s.Upstreams))
	require.True(t, upstreamsEqual(map[string]any{"upstreams": []any{map[string]any{"dial": "example.com:80"}}}, []string{"http://example.com"}))
	require.True(t, upstreamsEqual(map[string]any{"upstreams": []any{map[string]any{"dial": "example.com:443"}}, "transport": map[string]any{"tls": map[string]any{}}}, []string{"https://example.com"}))
	// Separate branches containing the host and target cannot be combined.
	configText := strings.Replace(configFor(s), `"host":["app.example.com"]`, `"host":["other.example.net"]`, 1)
	require.NoError(t, json.Unmarshal([]byte(configText), &config))
	require.False(t, containsSite(config, s, true))
	configText = strings.Replace(configFor(s), `"match":[{`, `"match":[{"method":["POST"],`, 1)
	require.NoError(t, json.Unmarshal([]byte(configText), &config))
	require.False(t, containsSite(config, s, true))
	// Generated Caddy routes put handlers under subroutes; propagate the match.
	require.NoError(t, json.Unmarshal([]byte(`{"apps":{"http":{"servers":{"example":{"routes":[{"match":[{"host":["app.example.com"]}],"handle":[{"handler":"subroute","routes":[{"match":[{"path":["/api*"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"192.0.2.80:8080"}]}]}]}]}]}}}}}`), &config))
	require.True(t, containsSite(config, s, true))
}
func TestPrepareTargetIdentityAndOwnershipGuards(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	text, _ := renderChecked(t, s)
	x.put("routes/foreign.caddy", text)
	r, err := readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Len(t, r.Routes, 2)
	require.Equal(t, "text-only", r.Routes[1].Class)
	q := x.request("create-route", s)
	for _, target := range []string{"../app.caddy", "/etc/caddy/app.caddy", "routes/hidden.secret.caddy", "elsewhere/app.caddy", "routes/app.txt", "Caddyfile"} {
		bad := q
		bad.Target = target
		_, err := prepareChecked(t, x.e, bad)
		require.Error(t, err, target)
	}
	for _, kind := range []string{"edit-route", "delete-route", "edit-service", "restore-owned"} {
		bad := q
		bad.Change.Kind = kind
		_, err := prepareChecked(t, x.e, bad)
		require.Error(t, err, kind)
	}
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input", "duplicate name")
	q.Target = "routes/foreign.caddy"
	q.Change.Kind = "edit-route"
	q.Change.RouteID = q.Target
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "source-read-only")
	require.NoError(t, x.d.DB.Exec("DELETE FROM proxy_verified_files").Error)
	x.owned("routes/app.caddy", text)
	q.Target = "routes/app.caddy"
	q.Change.RouteID = "wrong"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	q.Change.Kind = "create-route"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	q.Change.Kind = "edit-route"
	q.Change.RouteID = q.Target
	q.Change.Advanced = "invalid"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "invalid-input")
	x.put("routes/app.caddy", "app.example.com {\n respond unknown\n}\n")
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "unsupported-format")
	x.put("Caddyfile", strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n"))
	q.Change.Kind = "create-route"
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "import-missing")
}
func TestP6DeadlineBeginsAtActivation(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	data, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", AdminOff: true, Before: hostio.Response{Status: 404, Headers: map[string]string{"server": "Caddy"}}, NewSite: &s})
	i := proxy.Intent{Request: dto.ApplyRequest{ID: "deadline", Instance: x.i, Change: dto.Change{Kind: "create-route"}}, Engine: data}
	require.NoError(t, x.e.Reload(t.Context(), i))
	x.now = x.now.Add(2 * time.Second)
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, 50, x.waits)
	x.now = x.now.Add(time.Second)
	before := len(x.commands)
	o, err = x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Len(t, x.commands, before)
	require.Equal(t, dto.NoObservableDifference, o.Reason)
	require.NoError(t, x.e.Cleanup(t.Context(), i))
	_, ok := x.e.activated.Load(i.Request.ID)
	require.False(t, ok)
}
func TestObserveLogsAdminErrorsAndGatewayAreNeverSuccess(t *testing.T) {
	for _, which := range []string{"logs", "admin", "gateway", "redirect", "bad-json", "null", "config-status"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			x.e.wait = func(context.Context, time.Duration) error { x.now = x.now.Add(5 * time.Second); return nil }
			s := baseSite()
			data, _ := json.Marshal(state{Root: "/etc/caddy/Caddyfile", Admin: "localhost:2019", NewSite: &s})
			i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: "create-route"}}, Engine: data}
			x.execFn = func(args []string) hostio.ExecResult {
				if strings.Contains(args[len(args)-1], "/config/") {
					switch which {
					case "admin":
						return hostio.ExecResult{ExitCode: 1}
					case "redirect":
						r := httpResult(200, configFor(s))
						r.Stderr = []byte("  HTTP/1.1 301 Moved\n\n  HTTP/1.1 200 OK\n\n")
						return r
					case "bad-json":
						return httpResult(200, "bad")
					case "null":
						return httpResult(200, "null")
					case "config-status":
						return httpResult(404, "{}")
					default:
						return httpResult(200, configFor(s))
					}
				}
				if which == "gateway" {
					return httpResult(502, "gateway")
				}
				return httpResult(200, "changed")
			}
			if which == "logs" {
				x.c.HostConfig.LogConfig.Type = "none"
			}
			o, err := x.e.Observe(t.Context(), i, false)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			if which != "logs" && which != "gateway" {
				require.Equal(t, dto.AdminUnavailable, o.Reason)
			}
		})
	}
}

func TestInventoryRootImportAndMountGuards(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*dockerFixture)
		bad    bool
	}{
		{"route-directory-empty", func(x *dockerFixture) { x.i.RouteDirectory = "" }, true},
		{"route-directory-not-clean", func(x *dockerFixture) { x.i.RouteDirectory = "routes/../routes" }, true},
		{"route-directory-control", func(x *dockerFixture) { x.i.RouteDirectory = "routes\n" }, true},
		{"root-mismatch", func(x *dockerFixture) { x.i.RootFile = "/etc/caddy/other" }, true},
		{"missing-route-mount", func(x *dockerFixture) { x.i.RouteDirectory = "/outside/routes" }, true},
		{"import-arguments", func(x *dockerFixture) { x.put("Caddyfile", rootText+"import /etc/caddy/routes/*.caddy extra\n") }, true},
		{"placeholder-import", func(x *dockerFixture) { x.put("Caddyfile", rootText+"import /etc/caddy/{$IMPORT}\n") }, true},
		{"root-secret", func(x *dockerFixture) {
			x.c.Config.Cmd = []string{"run", "--config", "/etc/caddy/root.secret"}
			x.i.RootFile = "/etc/caddy/root.secret"
			x.put("root.secret", "secret payload")
		}, true},
		{"resume-admin-off", func(x *dockerFixture) {
			x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
			x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
		}, true}, // AUTO-3: read-only, the running configuration cannot be read
		{"relative-route-import", func(x *dockerFixture) {
			x.put("Caddyfile", strings.Replace(rootText, "/etc/caddy/routes/*.caddy", "routes/*.caddy", 1))
		}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			x := fixture(t)
			tt.mutate(x)
			inv, err := inventoryChecked(t, x.e, x.i)
			if tt.bad {
				require.True(t, err != nil || !inv.Eligibility.Writable, "must refuse bad inventory")
			} else {
				require.NoError(t, err)
				require.True(t, inv.Eligibility.Writable)
			}
		})
	}
}
func TestImportCyclesDepthAndStagingIsolation(t *testing.T) {
	for _, which := range []string{"cycle", "depth", "absolute", "glob-directory"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			switch which {
			case "cycle":
				x.put("Caddyfile", rootText+"import /etc/caddy/Caddyfile\n")
			case "depth":
				for n := 0; n < 10; n++ {
					x.put("import-"+string(rune('a'+n))+".caddy", "import /etc/caddy/import-"+string(rune('a'+n+1))+".caddy\n")
				}
				x.put("Caddyfile", rootText+"import /etc/caddy/import-a.caddy\n")
			case "absolute":
				x.put("extra.caddy", "extra.example.net {\n respond ok\n}\n")
				x.put("Caddyfile", rootText+"import /etc/caddy/extra.caddy\n")
			case "glob-directory":
				x.put("Caddyfile", rootText+"import /etc/caddy/*/*.caddy\n")
			}
			if which != "absolute" {
				_, err := readChecked(t, x.e, x.i)
				require.Error(t, err)
				if which == "depth" {
					require.ErrorContains(t, err, "input-limit")
				}
				if which == "cycle" {
					require.ErrorContains(t, err, "invalid-graph")
				}
				if which == "glob-directory" {
					require.ErrorContains(t, err, "unsupported-format")
				}
				return
			}
			q := x.request("create-route", baseSite())
			i, err := prepareChecked(t, x.e, q)
			require.NoError(t, err)
			i.StagingPath = "routes/.docklet-staging-example"
			require.NoError(t, validateChecked(t, x.e, i))
			require.NoError(t, x.d.Cleanup(t.Context(), i))
			x.noTransient()
		})
	}
}
func TestSiteStructuralGrammarRefusals(t *testing.T) {
	s := baseSite()
	s.TLS = "auto"
	s.Provider = "cloudflare"
	s.Credential = "DNS_API_TOKEN"
	s.CertificateDomain = "example.com"
	s.Path = "/api"
	s.Middlewares = []string{"compress", "strip-api", "custom-headers"}
	s.Upstreams = append(s.Upstreams, "http://192.0.2.81:8080")
	text, err := renderChecked(t, s)
	require.NoError(t, err)
	// Every structural line is required. Whitespace is the only normalization.
	for n, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.HasPrefix(line, "# docklet-certificate-domain") {
			continue
		}
		lines := strings.Split(strings.TrimSpace(text), "\n")
		lines[n] = "invalid_directive"
		_, err := parseChecked(t, strings.Join(lines, "\n"))
		require.Error(t, err, "line %d", n)
	}
	for _, pair := range [][2]string{{"# docklet-route: example-app", "# docklet-route: bad name"}, {"# docklet-certificate-domain: example.com", "# docklet-certificate-domain: bad"}, {"dns cloudflare {env.DNS_API_TOKEN}", "dns cloudflare {env.bad-token}"}, {"path /api*", "path /api"}, {"host app.example.com", "host other.example.net"}, {"lb_policy round_robin", "lb_policy random"}, {"reverse_proxy http://192.0.2.80:8080 http://192.0.2.81:8080 {", "reverse_proxy http://192.0.2.80:8080 {"}} {
		_, err := parseChecked(t, strings.Replace(text, pair[0], pair[1], 1))
		require.Error(t, err)
	}
	plain := baseSite()
	plain.TLS = "custom"
	plain.CertFile = "/certs/a.crt"
	plain.KeyFile = "/certs/a.key"
	custom, err := renderChecked(t, plain)
	require.NoError(t, err)
	for _, pair := range [][2]string{{"tls /certs/a.crt /certs/a.key", "tls /certs/a.crt"}, {"app.example.com {", "http://app.example.com {"}, {"tls /certs/a.crt /certs/a.key", "tls bad { secret }"}} {
		_, err := parseChecked(t, strings.Replace(custom, pair[0], pair[1], 1))
		require.Error(t, err)
	}
	auto := strings.Replace(text, "app.example.com, example.com, *.example.com {", "http://app.example.com, example.com, *.example.com {", 1)
	_, err = parseChecked(t, auto)
	require.Error(t, err)
}
