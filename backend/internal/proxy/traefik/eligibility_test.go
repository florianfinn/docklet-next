// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original independent eligibility and identity regressions.

package traefik

import (
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestCreateNameIsCheckedIndependentlyOfItsService(t *testing.T) {
	x := fixture(t)
	r := x.request("create-route")
	r.Target = "dynamic/new.yml"
	r.Change = dto.Change{Kind: "create-route", Name: "bad/name", Service: "backend", Rule: "Host(`new.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
	_, err := x.attempt(r)
	requireCode(t, err, dto.InvalidInput)
}
func TestCompletePlainMultilineRuleStillHasNoEditableSpan(t *testing.T) {
	source := strings.Replace(tiny, "rule: Host(`app.example.com`)", "rule: Host(`app.example.com`)\n        && PathPrefix(`/api`)", 1)
	_, err := parseDocument(source, "")
	requireCode(t, err, dto.UnsupportedFormat)
}
func TestV3ArgumentEOFAndRuneLiteralsAreRefused(t *testing.T) {
	for _, rule := range []string{"Host(", "Host('a')"} {
		require.NotPanics(t, func() { _, err := parseRule(rule); require.Error(t, err) })
	}
	_, err := parseRule("Header(`X-Example`, `end\\`)")
	require.NoError(t, err)
}
func TestOnlyKeysRejectsMissingAndNonmappingNodes(t *testing.T) {
	d, err := parseDocument("field: value\n", "")
	require.NoError(t, err)
	require.NotPanics(t, func() { require.False(t, onlyKeys(field(d.root, "missing"), "url")) })
	require.False(t, onlyKeys(field(d.root, "field"), "url"))
}
func TestTLSOptionsAndRenamedExistingUnknownReferencesAreChecked(t *testing.T) {
	source := strings.Replace(tiny, "      service: app", "      tls: {options: absent}\n      service: app", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	in := proxy.Intent{Request: dto.ApplyRequest{Target: "routes.yml"}}
	names, err := newUnresolved(map[string]*document{"routes.yml": d}, nil, dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "web"}}}, in)
	require.NoError(t, err)
	require.Contains(t, names, "absent")
	source = strings.Replace(tiny, "      service: app", "      middlewares: [existing@file]\n      service: app", 1)
	in.Old = proxy.Snapshot([]byte(source), true)
	in.Request.Change = dto.Change{Kind: "edit-route", RouteID: "routes.yml#app", Name: "renamed"}
	d, err = parseDocument(strings.Replace(source, "    app:\n", "    renamed:\n", 1), "")
	require.NoError(t, err)
	names, err = newUnresolved(map[string]*document{"routes.yml": d}, nil, dto.Inventory{EntryPoints: []dto.EntryPoint{{Name: "web"}}}, in)
	require.NoError(t, err)
	require.Empty(t, names)
}
func TestStagedValidationRechecksStaticInventoryAndGraphDefinitions(t *testing.T) {
	x := fixture(t)
	static := "providers: {file: {directory: /etc/traefik/dynamic, watch: true}}\nentryPoints: {web: {address: ':80'}}\n"
	x.put("traefik.yml", static)
	x.inspect.Config.Cmd = []string{"--configFile=/etc/traefik/traefik.yml"}
	in := x.prepare(x.request("edit-route"))
	require.NoError(t, x.d.Stage(t.Context(), in))
	x.put("traefik.yml", strings.Replace(static, "watch: true", "watch: false", 1))
	requireCode(t, x.validate(in), dto.WatchOff)
	x.put("traefik.yml", static)
	x.put("dynamic/duplicate.yml", tiny)
	requireCode(t, x.validate(in), dto.InvalidGraph)
}
func TestRouterRemovalRetainsAnUnindentedFullLineComment(t *testing.T) {
	source := strings.Replace(tiny, "      entryPoints:", "# Keep this explanation\n      entryPoints:", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var deletion edit
	require.NotPanics(t, func() { deletion, err = d.remove(at(d.root, "http", "routers"), 0); require.NoError(t, err) })
	out, err := d.apply([]edit{deletion})
	require.NoError(t, err)
	require.Contains(t, out, "    # Keep this explanation\n")
}
func TestRouteIDsRemainScopedWhenSourceAndNameContainSeparators(t *testing.T) {
	a, err := parseDocument(strings.ReplaceAll(tiny, "app", "name"), "")
	require.NoError(t, err)
	b, err := parseDocument(strings.Replace(tiny, "    app:\n", "    other.yml#name:\n", 1), "")
	require.NoError(t, err)
	routes := classify(map[string]*document{"routes.yml#other.yml": a, "routes.yml": b}, nil, dto.Inventory{})
	got, ok := findRoute(routes, "routes.yml#other.yml#name", "routes.yml#other.yml")
	require.True(t, ok)
	require.Equal(t, "routes.yml#other.yml", got.File)
	require.Equal(t, "name", got.Name)
}

func TestEmptyMultilineTokensAreNotEditable(t *testing.T) {
	for _, style := range []string{"|", ">"} {
		source := "http:\n  middlewares:\n    h:\n      headers:\n        customResponseHeaders:\n          X-Empty: " + style
		_, err := parseDocument(source, "")
		requireCode(t, err, dto.UnsupportedFormat)
	}
}
func TestUnsupportedServiceShapesDoNotInventUpstreams(t *testing.T) {
	mixed := tiny + "      weighted: {services: [{name: other}]}\n"
	d, err := parseDocument(mixed, "")
	require.NoError(t, err)
	require.Equal(t, "text-only", classify(map[string]*document{"routes.yml": d}, nil, dto.Inventory{})[0].Class)
	source := strings.Replace(tiny, "servers: [{url: 'http://192.0.2.1:8080'}]", "servers: {url: {url: 'http://192.0.2.1:8080'}}", 1)
	d, err = parseDocument(source, "")
	require.NoError(t, err)
	routes := classify(map[string]*document{"routes.yml": d}, nil, dto.Inventory{})
	require.Equal(t, "text-only", routes[0].Class)
	require.Empty(t, routes[0].Upstreams)
}
func TestStructuralReplacementRetainsCommentsAndRejectsNestedFlowRemoval(t *testing.T) {
	source := strings.Replace(tiny, "      service: app", "      middlewares:\n        - auth # Retained\n      service: app", 1)
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "routers", "app"), map[string]any{"middlewares": []string{}}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "- auth # Retained\n", "[]\n        # Retained\n", 1), out)
	d, err = parseDocument("field:\n  - first\n", "")
	require.NoError(t, err)
	edits = nil
	require.NoError(t, d.patch(d.root, map[string]any{"field": map[string]any{"key": "value"}}, &edits))
	out, err = d.apply(edits)
	require.NoError(t, err)
	next, err := parseDocument(out, "")
	require.NoError(t, err)
	require.Equal(t, "!!map", field(next.root, "field").Tag)
	d, err = parseDocument(tiny, "")
	require.NoError(t, err)
	edits = nil
	require.Error(t, d.patch(at(d.root, "http", "services", "app", "loadBalancer"), map[string]any{"servers": []any{map[string]any{"url": nil}}}, &edits))
}

func TestPlainScalarQuoteCharactersDoNotHideTheRealComment(t *testing.T) {
	source := "http:\n  middlewares:\n    h:\n      headers:\n        customResponseHeaders:\n          Keep: value\n          Remove: a\"b\"'c' # Retained\n"
	d, err := parseDocument(source, "")
	require.NoError(t, err)
	var edits []edit
	require.NoError(t, d.patch(at(d.root, "http", "middlewares", "h", "headers", "customResponseHeaders"), map[string]any{"Remove": nil}, &edits))
	out, err := d.apply(edits)
	require.NoError(t, err)
	require.Contains(t, out, "          # Retained\n")
	require.NotContains(t, out, "a\"b\"'c'")
}
