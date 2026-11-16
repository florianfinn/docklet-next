// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine tests; adopted from the S3 fix verification counterexamples.

package traefik

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestVerifyCaseDistinctSiblingDoesNotReject(t *testing.T) {
	for _, line := range []string{
		`{"level":"error","routerName":"App@file","error":"middleware \"gone@file\" does not exist"}`,
		`2026-11-16T01:00:00Z ERR error="middleware gone@file does not exist" routerName=App@file`,
	} {
		t.Run(line, func(t *testing.T) {
			x := fixture(t)
			sibling := strings.ReplaceAll(tiny, "app", "App")
			sibling = strings.Replace(sibling, "      service: App\n", "      service: App\n      middlewares: [gone@file]\n", 1)
			x.put("dynamic/sibling.yml", sibling)
			r := x.request("edit-route")
			r.Change.Rule = "Host(`next.example.com`)"
			in := x.prepare(r)
			s, err := state(in)
			require.NoError(t, err)
			require.Contains(t, s.Scope, "app@file")
			require.NotContains(t, s.Scope, "App@file")
			x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
			x.logs = in.PublishedAt.Format(time.RFC3339Nano) + " " + line + "\n"
			obs, err := x.e.Observe(t.Context(), in, false)
			require.NoError(t, err)
			require.False(t, obs.Restore, "an unrelated case-distinct sibling must not restore: %+v", obs)
			require.Equal(t, dto.Verified, obs.State)
		})
	}
}

func TestVerifyScopeCoversOldNewAndReferences(t *testing.T) {
	oldText := strings.Replace(tiny, "      service: app", "      middlewares: [old-mw]\n      service: old-service", 1)
	nextText := strings.Replace(tiny, "    app:\n", "    renamed:\n", 1)
	nextText = strings.Replace(nextText, "      service: app", "      middlewares: [new-mw, external@docker]\n      service: new-service", 1)
	d, err := parseDocument(nextText, "routes.yml")
	require.NoError(t, err)
	scope := logScope(proxy.Snapshot([]byte(oldText), true), d, "routes.yml")
	for _, name := range []string{"app@file", "renamed@file", "old-mw@file", "new-mw@file", "old-service@file", "new-service@file", "external@docker"} {
		require.Contains(t, scope, name)
		for _, line := range []string{
			`{"level":"error","routerName":"` + name + `","error":"build failed"}`,
			`ERR error="build failed" serviceName=` + name,
			`ERR error="build failed" middlewareName=` + name,
		} {
			require.True(t, providerError(line, scope), line)
		}
	}
	for _, line := range []string{
		`{"level":"error","providerName":"dns-primary.acme","routerName":"app@file","error":"failed"}`,
		`ERR error="failed" providerName=docker routerName=app@file`,
		`{"level":"error","routerName":"other@file","error":"missing other@file"}`,
	} {
		require.False(t, providerError(line, scope), line)
	}
}

func TestVerifyHiddenLateSiblingValidateAndGraph(t *testing.T) {
	for _, p := range []string{"dynamic/.late.yml", "dynamic/.hidden/late.yaml", "dynamic/.late.toml"} {
		t.Run(p, func(t *testing.T) {
			x := fixture(t)
			r := x.request("edit-route")
			r.Change.Priority = new(9)
			in := x.prepare(r)
			require.NoError(t, x.d.Stage(t.Context(), in))
			content := "http: ["
			if strings.HasSuffix(p, ".toml") {
				content = "[http.routers]\n"
			}
			x.put(p, content)
			hashes, err := x.d.Graph(t.Context(), x.i)
			require.NoError(t, err)
			require.Contains(t, hashes, p)
			require.Error(t, x.validate(in))
			require.Error(t, x.d.Publish(t.Context(), in))
			require.Equal(t, tiny, x.read(r.Target))
		})
	}
}

func TestVerifyEmptyTargetNeverPatches(t *testing.T) {
	for _, text := range []string{"", "# placeholder\n"} {
		x := fixture(t)
		x.put("dynamic/routes.yml", text)
		r := x.request("edit-route")
		r.Change.Priority = new(5)
		_, err := x.attempt(r)
		require.Error(t, err)
		require.Equal(t, text, x.read(r.Target))
	}
}

func TestVerifyMarker404CannotBeLoadEvidence(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	x.probeFunc = func([]string) hostio.ExecResult { return response(404, "") }
	obs, err := x.e.Observe(t.Context(), in, true)
	require.NoError(t, err)
	require.NotEqual(t, dto.Verified, obs.State)
	require.Empty(t, obs.Evidence)
}

func TestVerifyOpaqueIgnoresRouterAndOtherProviders(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/extra.secret.yml", "opaque")
	x.logs = epoch.Add(time.Minute).Format(time.RFC3339Nano) + ` {"level":"error","routerName":"app@file","error":"failed"}` + "\n"
	r := x.request("edit-route")
	r.Change.Priority = new(5)
	_, err := x.attempt(r)
	require.NoError(t, err)
}

func TestVerifyScopeSurvivesIntentEncoding(t *testing.T) {
	x := fixture(t)
	in := x.prepare(x.request("edit-route"))
	b, err := json.Marshal(in)
	require.NoError(t, err)
	var again proxy.Intent
	require.NoError(t, json.Unmarshal(b, &again))
	before, err := state(in)
	require.NoError(t, err)
	after, err := state(again)
	require.NoError(t, err)
	require.Equal(t, before.Scope, after.Scope)
	require.NotEmpty(t, after.Scope)
}

func TestVerifyMatcherAliasesKeepInfrastructureClassification(t *testing.T) {
	for _, rule := range []string{"PathPrefix(`/`)", "pathprefix(`/`)", "PATHPREFIX(`/`)", "Pathprefix(`/`)", "HostRegexp(`.*`)", "hostregexp(`.*`)", "HOSTREGEXP(`.*`)", "Hostregexp(`.*`)"} {
		t.Run(rule, func(t *testing.T) {
			x := fixture(t)
			x.put("dynamic/routes.yml", strings.Replace(tiny, "Host(`app.example.com`)", rule, 1))
			read, err := x.readResult()
			require.NoError(t, err)
			require.True(t, read.Eligibility.Writable)
			route, ok := findRoute(read.Routes, "app", "dynamic/routes.yml")
			require.True(t, ok)
			if route.Class != "infrastructure" {
				t.Errorf("catch-all classified %s for %s", route.Class, rule)
			}
			r := x.request("edit-route")
			r.Change.Priority = new(5)
			_, err = x.attempt(r)
			require.Error(t, err, "infrastructure must stay outside the editable form")
		})
	}
}
