// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"os"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP26V3RuleGrammar(t *testing.T) {
	valid := []string{"Host(`app.example.com`)", "HostRegexp(`^[a-z0-9-]+\\.example\\.com$`)", "Path(`/exact`)", "PathPrefix(`/api`)", "PathRegexp(`^/v[0-9]+/`)", "Method(`GET`)", "Header(`X-Example`, `on`)", "HeaderRegexp(`X-Example`, `^on$`)", "Query(`enabled`)", "Query(`enabled`, `yes`)", "QueryRegexp(`enabled`, `yes|on`)", "ClientIP(`192.0.2.0/24`)", "ClientIP(`2001:db8::1`)", `!(Host("a.example.com") || Host("b.example.net")) && (PathPrefix("/api") || !Method("POST"))`}
	for _, rule := range valid {
		t.Run("valid/"+rule, func(t *testing.T) { _, err := parseRule(rule); require.NoError(t, err) })
	}
	invalid := []string{"", "Unknown(`value`)", "Headers(`X-Example`, `on`)", "Host(`a.example.com`, `b.example.com`)", "HostRegexp(`{subdomain:[a-z]+}.example.com`)", "PathRegexp(`[`)", "HeaderRegexp(`X`, `[`)", "ClientIP(`bad`)", "Path(`relative`)", "Host('app.example.com')", "Host(`app.example.com`) &&", "(Host(`app.example.com`)", "Host(`app.example.com`))", "Host(`app.example.com`) + Path(`/`)", "Host(\"bad\\q\")", "Host(`a\nexample.com`)", "Header(`X`)", "ClientIP(`999.0.0.1`)", "Host(`a.example.com`) trailing", "!"}
	for _, rule := range invalid {
		t.Run("invalid/"+rule, func(t *testing.T) {
			_, err := parseRule(rule)
			require.Error(t, err, "invalid matcher must not publish")
		})
	}
}
func shared(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/shared.yml")
	require.NoError(t, err)
	return string(b)
}
func TestP15GoldenBytesPerEditKind(t *testing.T) {
	source := shared(t)
	cases := []struct {
		name, router string
		patch        map[string]any
		from, to     string
	}{
		{"rule", "app-one", map[string]any{"rule": `Host("new.example.com")`}, `Host("app.example.com")`, `Host("new.example.com")`},
		{"listener", "app-one", map[string]any{"entryPoints": []string{"internal-https"}}, "entryPoints: [websecure] # Public", "entryPoints: [internal-https] # Public"},
		{"middleware", "app-one", map[string]any{"middlewares": []string{"headers", "other@file"}}, "middlewares: [auth, other@file]", "middlewares: [headers, other@file]"},
		{"service", "app-one", map[string]any{"service": "replacement"}, "service: app-one\n", "service: replacement\n"},
		{"resolver", "wildcard", map[string]any{"tls": map[string]any{"certResolver": "dns-secondary"}}, "certResolver: dns-primary", "certResolver: dns-secondary"},
		{"priority", "priority", map[string]any{"priority": 101}, "priority: 100 #", "priority: 101 #"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parseDocument(source, "routes.yml")
			require.NoError(t, err)
			var edits []edit
			require.NoError(t, d.patch(at(d.root, "http", "routers", tt.router), tt.patch, &edits))
			out, err := d.apply(edits)
			require.NoError(t, err)
			require.Equal(t, strings.Replace(source, tt.from, tt.to, 1), out, "only the intended span changes")
		})
	}
	t.Run("upstream", func(t *testing.T) {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		var edits []edit
		require.NoError(t, d.patch(at(d.root, "http", "services", "app-one", "loadBalancer"), map[string]any{"servers": servers([]string{"http://192.0.2.21:8080"})}, &edits))
		out, err := d.apply(edits)
		require.NoError(t, err)
		require.Equal(t, strings.Replace(source, "http://192.0.2.11:8080", "http://192.0.2.21:8080", 1), out)
	})
	t.Run("noop", func(t *testing.T) {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		for _, key := range keys(at(d.root, "http", "routers")) {
			var edits []edit
			n := at(d.root, "http", "routers", key)
			require.NoError(t, d.patch(n, value(n).(map[string]any), &edits))
			out, err := d.apply(edits)
			require.NoError(t, err)
			require.Equal(t, source, out)
		}
	})
	t.Run("delete", func(t *testing.T) {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		routers := at(d.root, "http", "routers")
		for j := 0; j < len(routers.Content); j += 2 {
			if routers.Content[j].Value == "app-two" {
				ed, err := d.remove(routers, j)
				require.NoError(t, err)
				out, err := d.apply([]edit{ed})
				require.NoError(t, err)
				start := strings.Index(source, "    app-two:")
				end := strings.Index(source, "    app-three:")
				require.Equal(t, source[:start]+source[end:], out)
			}
		}
	})
}
func TestSpanRefusalsAndStructuralEdits(t *testing.T) {
	for _, s := range []string{"http: &defaults {}\ncopy: *defaults\n", "http: !!map {}\n", "http: {}\nhttp: {}\n", "http: [", "[]", "http: {}\n---\nhttp: {}", "? [a,b]\n: value"} {
		_, err := parseDocument(s, "")
		require.Error(t, err, s)
	}
	d, err := parseDocument(strings.ReplaceAll(tiny, "\n", "\r\n"), "")
	require.Error(t, err)
	d, err = parseDocument("http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app}}}\n", "")
	require.NoError(t, err)
	_, err = d.remove(at(d.root, "http", "routers"), 0)
	require.Error(t, err)
	d, err = parseDocument(tiny, "")
	require.NoError(t, err)
	ed, err := d.remove(at(d.root, "http", "routers"), 0)
	require.NoError(t, err)
	next, err := d.apply([]edit{ed})
	require.NoError(t, err)
	parsed, err := parseDocument(next, "")
	require.NoError(t, err)
	require.Empty(t, keys(at(parsed.root, "http", "routers")))
	require.NotNil(t, at(parsed.root, "http", "services", "app"))
	d, err = parseDocument(tiny, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"priority": 10, "middlewares": []string{"headers"}}, &edits))
	next, err = d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, next, "      priority: 10\n")
	require.Contains(t, next, "      middlewares:\n        - headers\n")
}
func TestClassifierPrototypeParityAndAcceptedExtensions(t *testing.T) {
	d, err := parseDocument(shared(t), "routes.yml")
	require.NoError(t, err)
	other, err := parseDocument("http:\n  middlewares:\n    other:\n      headers: {}\n", "")
	require.NoError(t, err)
	docs := map[string]*document{"routes.yml": d, "other.yml": other}
	opaque := []dto.OpaqueFile{{Path: "auth.secret.yml"}}
	routes := classify(docs, opaque, dto.Inventory{Providers: []string{"file", "docker"}})
	require.Len(t, routes, 11)
	for _, r := range routes {
		if r.Name == "redirect" {
			require.Equal(t, "infrastructure", r.Class)
		} else {
			require.Equal(t, "editable", r.Class, r.Name)
		}
	}
	app, _ := findRoute(routes, "app-one", "routes.yml")
	require.Equal(t, []dto.Reference{{Name: "auth", Kind: "local", File: "routes.yml"}, {Name: "other@file", Kind: "local", File: "other.yml"}}, app.References)
	plugin, _ := findRoute(routes, "plugin-route", "routes.yml")
	require.Equal(t, []dto.Reference{{Name: "security-plugin", Kind: "plugin", File: "routes.yml"}, {Name: "private-auth@file", Kind: "opaque", File: "auth.secret.yml"}, {Name: "foreign@docker", Kind: "external"}}, plugin.References)
	appTwo, _ := findRoute(routes, "app-two", "routes.yml")
	require.Len(t, appTwo.Dependants, 3)
	for _, tt := range []struct {
		from, to string
		class    string
	}{
		{"service: app-one", "service: missing@docker", "text-only"},
		{"service: app-one", "service: app-one\n      observability: {}", "text-only"},
		{"tls: {} # Uses", "tls: {options: strict} # Uses", "text-only"},
		{"servers: [{url: 'http://192.0.2.11:8080'}]", "servers: [{url: 'http://192.0.2.11:8080', weight: 2}]", "text-only"},
		{`rule: 'Host("app.example.com")'`, `rule: Path(` + "`/exact`" + `)`, "editable"},
		{`rule: 'Host("app.example.com")'`, `rule: Host(` + "`app.example.com`" + `) || Host(` + "`other.example.net`" + `)`, "editable"},
		{`rule: 'Host("app.example.com")'`, `rule: HostRegexp(` + "`.*example.com`" + `)`, "editable"},
		{`rule: 'Host("app.example.com")'`, `rule: Header(` + "`X-Example`" + `, ` + "`on`" + `)`, "editable"},
	} {
		t.Run(tt.to, func(t *testing.T) {
			doc, err := parseDocument(strings.Replace(shared(t), tt.from, tt.to, 1), "")
			require.NoError(t, err)
			r, _ := findRoute(classify(map[string]*document{"routes.yml": doc}, opaque, dto.Inventory{}), "app-one", "routes.yml")
			require.Equal(t, tt.class, r.Class)
		})
	}
}

func TestRuleAndSourceLimits(t *testing.T) {
	for _, s := range []string{strings.Repeat("!", 130) + "Host(`app.example.com`)", strings.Repeat(" ", 16385), "Host(`app.example.com`", "Host(\"unterminated)", "Header(`x` `y`)", "Host(`a\x00b`)"} {
		require.NotPanics(t, func() { _, err := parseRule(s); require.Error(t, err) })
	}
	for _, s := range []string{"http: null\n", "http: !!str x\n", "http: &base {}\n", "http: {routers: {a: {rule: |\n  text}}}", "http: {routers: {app: {rule: 'a'\n service: app}}}", "http:\n  routers:\n    app:\n      rule: Host(`app.example.com`)\n        && PathPrefix(`/`)\n"} {
		require.NotPanics(t, func() {
			_, err := parseDocument(s, "")
			if err == nil {
				d, _ := parseDocument(s, "")
				err = validateDocument(d, "")
			}
			require.Error(t, err)
		})
	}
}
func TestSchemaWrongFieldTypes(t *testing.T) {
	for _, s := range []string{
		"http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app, entryPoints: [1]}}}",
		"http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app, middlewares: auth}}}",
		"http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app, priority: many}}}",
		"http: {routers: {app: {rule: 'Host(`app.example.com`)', service: app, tls: true}}}",
		"http: {routers: {app: {rule: 'Host(`app.example.com`)'}}}",
		"http: {routers: {app: {service: app}}}",
		"http: {services: {app: {loadBalancer: {servers: [7]}}}}",
		"http: {services: {app: {loadBalancer: {servers: [{url: 'ftp://example.com'}]}}}}",
		"http: {services: {app: {}}}",
		"http: {middlewares: {m: {headers: {frameDeny: 'yes'}}}}",
		"http: {middlewares: {m: {headers: {}, compress: {}}}}",
		"http: {middlewares: {m: {plugin: {p: true}}}}",
		"http: {middlewares: {m: {unknown: {}}}}",
		"tls: {options: {strict: {sniStrict: 'yes'}}}",
		"tls: {certificates: [{certFile: 7}]}",
	} {
		t.Run(s, func(t *testing.T) {
			require.NotPanics(t, func() {
				d, err := parseDocument(s, "candidate.yml")
				require.NoError(t, err)
				require.Error(t, validateDocument(d, "candidate.yml"))
			})
		})
	}
	for _, u := range []string{"", "ftp://example.com", "http://", "http://user@example.com", "http://example.com/#fragment", "http://example.com/\n"} {
		require.NotPanics(t, func() { require.False(t, validURL(u), u) })
	}
}

func TestBlockListResizePreservesCommentPositions(t *testing.T) {
	source := strings.Replace(tiny, "entryPoints: [web]", "entryPoints:\n        - web # First listener\n        # Listener order\n        - websecure # Second listener", 1)
	for _, tt := range []struct {
		items []string
		want  string
	}{
		{[]string{"internal", "websecure", "extra"}, strings.Replace(source, "- web # First", "- internal # First", 1)},
		{[]string{"internal"}, strings.Replace(strings.Replace(source, "- web # First", "- internal # First", 1), "        - websecure # Second listener\n", "        # Second listener\n", 1)},
	} {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		var edits []edit
		require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"entryPoints": tt.items}, &edits))
		out, err := d.apply(edits)
		require.NoError(t, err)
		if len(tt.items) == 3 {
			tt.want = strings.Replace(tt.want, "        - websecure # Second listener\n", "        - websecure # Second listener\n        - extra\n", 1)
		}
		require.Equal(t, tt.want, out)
	}
}

func TestNoRouteHeadersAndResponseClasses(t *testing.T) {
	require.True(t, noRoute(hostio.Response{Status: 404}))
	require.False(t, noRoute(hostio.Response{Status: 404, Headers: map[string]string{"content-length": "19", "content-type": "text/plain"}}), "a 404 with headers is not BusyBox's no-route shape")
	require.False(t, noRoute(hostio.Response{Status: 404, BodyHash: "a"}))
	require.False(t, noRoute(hostio.Response{Status: 404, Followed: true}))
	require.False(t, noRoute(hostio.Response{Status: 200}))
	require.False(t, routingEvidence("edit-route", hostio.Response{}, hostio.Response{Status: 200}))
	require.False(t, routingEvidence("edit-route", hostio.Response{Status: 200}, hostio.Response{}))
}

func TestProbeSelectionAndProviderLogCorpus(t *testing.T) {
	inv := dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "web", Address: ":80"}, {Name: "secure", Address: ":443", TLS: true}, {Name: "dns", Address: ":53/udp"}}}
	probes := routeProbes(dto.Route{ID: "a", Rule: "Host(`app.example.com`) && PathPrefix(`/api`)"}, inv)
	require.Len(t, probes, 2)
	require.Equal(t, "http://127.0.0.1:80/api", probes[0].URL)
	require.Equal(t, "https://127.0.0.1:443/api", probes[1].URL)
	probes = routeProbes(dto.Route{ID: "a", Rule: "Host(`app.example.com`)", EntryPoints: []string{"secure"}}, inv)
	require.Len(t, probes, 1)
	require.Equal(t, "https://127.0.0.1:443/", probes[0].URL)
	probes = routeProbes(dto.Route{Rule: "HostRegexp(`^[a-z0-9-]+\\.example\\.com$`)"}, inv)
	require.Equal(t, "docklet-probe.example.com", probes[0].Host)
	for _, rule := range []string{"Unknown(`a`)", "HostRegexp(`.*`)", "HostRegexp(`^[a-z0-9-]+\\.example\\.com`)", "HostRegexp(`abc$`)"} {
		probes = routeProbes(dto.Route{Rule: rule}, inv)
		require.NotEmpty(t, probes)
		require.Equal(t, dto.NotObservable, probes[0].Reason)
	}
	probes = routeProbes(dto.Route{Rule: "Host(`app.example.com`)"}, dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "bad", Address: "invalid"}}})
	require.NotEmpty(t, probes)
	require.Equal(t, dto.NotObservable, probes[0].Reason)
	marker := markerProbe("a", inv)
	require.Equal(t, "http://127.0.0.1:80/", marker.URL)
	marker = markerProbe("a", dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "dns", Address: ":53/udp"}}})
	require.Equal(t, dto.NotObservable, marker.Reason)
	for _, line := range []string{
		`{"error":"bad","providerName":"file"}`, `time ERR rejected routerName=app@file`, `level=error msg=rejected routerName=app@file`, `{"level":"error","message":"bad","routerName":"app@file"}`, `ERR rejected routerName=app@file`, `time error="bad file provider"`, `time error="bad configuration file"`,
	} {
		require.True(t, providerError(line, []string{"app@file"}), line)
	}
	for _, line := range []string{`INF Configuration loaded providerName=file`, `{"level":"info","message":"Configuration loaded","providerName":"file"}`, `ERR error="unrelated docker failure" providerName=docker`,
		// Measured B0 record: ACME issuance of an unrelated router fails after every reload.
		measuredACME, `{"level":"error","routerName":"other@file","error":"middleware \"gone@file\" does not exist"}`, `ERR rejected routerName=myapp@file`, `ERR rejected routerName=app@file-old`,
	} {
		require.False(t, providerError(line, []string{"app@file"}), line)
	}
	require.False(t, providerError(`{"level":"error","routerName":"app@file","error":"bad"}`, nil), "the opaque health check counts aggregate file-provider errors only")
}

func TestNoRouteCannotVerifyChangedRouting(t *testing.T) {
	missing := hostio.Response{Status: 404}
	backend := hostio.Response{Status: 404, Headers: map[string]string{"content-length": "42", "content-type": "text/plain"}}
	require.False(t, noRoute(hostio.Response{Status: 200}))
	require.False(t, routingEvidence("create-route", missing, missing))
	require.False(t, routingEvidence("delete-route", hostio.Response{}, missing))
	require.False(t, routingEvidence("delete-route", missing, missing))
	require.False(t, routingEvidence("delete-route", backend, backend))
	require.False(t, routingEvidence("edit-route", hostio.Response{Status: 200}, missing))
}

func TestListGrowthAndOptionalFieldAtSameOffset(t *testing.T) {
	source := strings.Replace(tiny, "      entryPoints: [web]\n      service: app\n", "      service: app\n      middlewares:\n        - auth # Handler\n", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"middlewares": []string{"headers", "auth"}, "priority": 10}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	expected := strings.Replace(source, "        - auth # Handler\n", "        - headers # Handler\n        - auth\n      priority: 10\n", 1)
	require.Equal(t, expected, out)
}

func TestStaticFieldFoldingPreservesOnlyDictionaryNames(t *testing.T) {
	for input, want := range map[string]string{"EXPERIMENTAL.PLUGINS.PluginCase.ModuleName": "experimental.plugins.PluginCase.modulename", "OTHER.PLUGINS.Name.Field": "other.plugins.name.field", "EXPERIMENTAL.OTHER.Name.Field": "experimental.other.name.field"} {
		require.Equal(t, want, optionKey(input))
	}
}

func TestRuleSyntaxVersions(t *testing.T) {
	for _, syntax := range []string{"v2", "v3", "v4"} {
		d, err := parseDocument(strings.Replace(tiny, "      service: app", "      ruleSyntax: "+syntax+"\n      service: app", 1), "")
		require.NoError(t, err)
		err = validateDocument(d, "")
		if syntax == "v4" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestPlainQuotesAndUnicodeInFlowCollectionsStayUntouched(t *testing.T) {
	source := tiny + "  middlewares:\n    quote:\n      headers:\n        customRequestHeaders: {X-Example: Grüße, X-Quote: a\"b, X-Single: a'b}\n"
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	require.NoError(t, validateDocument(d, ""))
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"priority": 10}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "      service: app\n", "      service: app\n      priority: 10\n", 1), out)
}

func TestNestedTLSAndStructuralCommentParity(t *testing.T) {
	source := shared(t)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	patch := map[string]any{"tls": map[string]any{"certResolver": "dns-secondary", "domains": []any{map[string]any{"main": "example.net", "sans": []string{"*.example.net"}}}}}
	require.NoError(t, d.patch(at(d.root, "http", "routers", "wildcard"), patch, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	want := strings.Replace(strings.Replace(strings.Replace(source, "certResolver: dns-primary", "certResolver: dns-secondary", 1), "main: example.com", "main: example.net", 1), "sans: ['*.example.com']", "sans: ['*.example.net']", 1)
	require.Equal(t, want, out)
	for _, patch := range []map[string]any{{"tls": map[string]any{}}, {"middlewares": nil, "tls": nil}} {
		d, err := parseDocument(source, "")
		require.NoError(t, err)
		var edits []edit
		key := "wildcard"
		if len(patch) > 1 {
			key = "app-one"
		}
		require.NoError(t, d.patch(at(d.root, "http", "routers", key), patch, &edits))
		out, err := d.apply(edits)
		require.NoError(t, err)
		commentLines := func(text string) []string {
			a := []string{}
			for _, line := range strings.Split(text, "\n") {
				if k := strings.IndexByte(line, '#'); k >= 0 {
					a = append(a, line[k:])
				}
			}
			return a
		}
		require.Equal(t, commentLines(source), commentLines(out))
		require.NoError(t, validateDocument(mustDocument(t, out), ""))
	}
}
func mustDocument(t *testing.T, s string) *document {
	t.Helper()
	d, err := parseDocument(s, "")
	require.NoError(t, err)
	return d
}
func TestFlowMapFieldSplicesAndCommentedSequenceResize(t *testing.T) {
	source := `http: {routers: {app: {rule: 'Host("app.example.com")', service: app}}, services: {app: {loadBalancer: {servers: [{url: 'http://192.0.2.1:8080'}]}}}}` + "\n"
	d := mustDocument(t, source)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"service": "changed", "middlewares": []string{"auth"}}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, out, `rule: 'Host("app.example.com")', service: changed`)
	require.Contains(t, out, "middlewares: [auth]")
	source = strings.Replace(tiny, "servers: [{url: 'http://192.0.2.1:8080'}]", "servers: [{url: 'http://192.0.2.1:8080'}, # First upstream\n                  {url: 'http://192.0.2.2:8080'}] # Service note", 1)
	d = mustDocument(t, source)
	seq := at(d.root, "http", "services", "app", "loadBalancer", "servers")
	sp := d.spans[seq]
	edits = nil
	require.NoError(t, d.replace(seq, servers([]string{"http://192.0.2.3:8080"}), &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, source[:sp.start]))
	require.True(t, strings.HasSuffix(out, source[sp.end:]))
	require.Equal(t, 1, strings.Count(out, "# First upstream"))
	require.Equal(t, 1, strings.Count(out, "# Service note"))
	require.Less(t, strings.Index(out, "# First upstream"), strings.Index(out, "# Service note"))
	require.NoError(t, validateDocument(mustDocument(t, out), ""))
}
func TestListGrowthAtEOFAndQuotedCommentTokens(t *testing.T) {
	source := "http:\n  routers:\n    app:\n      rule: Host(`app.example.com`)\n      service: app\n      middlewares:\n        - auth"
	d := mustDocument(t, source)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"middlewares": []string{"headers", "auth"}, "priority": 10}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	next := mustDocument(t, out)
	require.Equal(t, []string{"headers", "auth"}, list(at(next.root, "http", "routers", "app", "middlewares")))
	require.Contains(t, out, "priority: 10")
	require.True(t, strings.HasPrefix(out, source[:strings.Index(source, "        - auth")]))
	source = strings.Replace(tiny, "      entryPoints: [web]", "      middlewares:\n        - 'auth # data' # First handler\n        - \"auth\\\"data\" # Second handler", 1)
	d = mustDocument(t, source)
	edits = nil
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"middlewares": []string{"headers"}}, &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	require.NotContains(t, out, "# data")
	require.Equal(t, 1, strings.Count(out, "# First handler"))
	require.Equal(t, 1, strings.Count(out, "# Second handler"))
}

func TestPinnedTraefikMatcherArityAndASCIIParity(t *testing.T) {
	for _, rule := range []string{"Method(`get`)", "Method(`M-SEARCH`)", "QueryRegexp(`enabled`)", "Host(`*`)", "Host(`[2001:db8::1]`)"} {
		_, err := parseRule(rule)
		require.NoError(t, err, rule)
	}
	for _, rule := range []string{"Host(`ä.example.com`)", "HostRegexp(`ä.*`)", "ClientIP(`2001:db8::1%eth0`)", "Method(``)", "Host(``)"} {
		_, err := parseRule(rule)
		require.Error(t, err, rule)
	}
	for _, tt := range []struct{ rule, class string }{{"Host(`app.example.com`) && HostRegexp(`.*`)", "editable"}, {"Host(`*`)", "infrastructure"}, {"HostRegexp(\"^.*$\")", "infrastructure"}} {
		doc := mustDocument(t, strings.Replace(tiny, "Host(`app.example.com`)", tt.rule, 1))
		routes := classify(map[string]*document{"routes.yml": doc}, nil, dto.Inventory{})
		require.Equal(t, tt.class, routes[0].Class)
	}
}

func TestSyntaxAndDuplicateDiagnosticsKeepTheirActualLine(t *testing.T) {
	for _, text := range []string{"# heading\nhttp:\n  routers: ]\n", "# heading\nhttp: {}\nhttp: {}\n"} {
		_, err := parseDocument(text, "bad.yml")
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		require.Equal(t, "bad.yml", p.Path)
		require.Equal(t, 3, p.Line)
		require.NotEmpty(t, p.Detail)
	}
}
