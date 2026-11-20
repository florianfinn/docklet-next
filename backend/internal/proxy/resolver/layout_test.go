// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/traefik"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func request() dto.ResolverRequest {
	return dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "Example", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "custom", URL: "https://ca.example.test/directory"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "http", EntryPoint: "web"}}, Variables: []dto.ResolverVariable{}}
}

const composeText = "# operator heading\nservices:\n  gateway:\n    image: traefik:v3.7.16 # pinned image\n    command:\n      - --entrypoints.web.address=:80 # listener\n      - --providers.file.directory=/etc/traefik/routes\n    environment:\n      INLINE: existing-inline-sentinel\n  app:\n    image: example/app:1.0.0\n# operator footer\n"

func target(t *testing.T, static bool) resolver.Target {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeText), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("REUSED='existing-reused-sentinel'\r\n# untouched"), 0600))
	out := resolver.Target{ProjectID: "project-one", Directory: dir, Project: "example", Service: "gateway", Compose: "compose.yaml"}
	if static {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "static.yaml"), []byte("# static heading\nentryPoints:\n  web:\n    address: ':80'\n# static footer\n"), 0644))
		out.Static = "config/static.yaml"
		f, e := resolver.Read(dir, out.Static)
		require.NoError(t, e)
		f.Kind = "static"
		out.Files = append(out.Files, f)
	}
	for _, p := range []string{"compose.yaml", ".env"} {
		f, e := resolver.Read(dir, p)
		require.NoError(t, e)
		if p == ".env" {
			f.Kind = "env"
		} else {
			f.Kind = "compose"
		}
		out.Files = append(out.Files, f)
	}
	return out
}
func inventory() dto.Inventory {
	return dto.Inventory{Eligibility: dto.Eligibility{Writable: true}, EntryPoints: []dto.EntryPoint{{Name: "web", Address: ":80"}}, Resolvers: []string{"example"}}
}
func build(t *testing.T, v resolver.Target, q dto.ResolverRequest) resolver.Target {
	t.Helper()
	out, e := resolver.Build(t.Context(), v, q, inventory(), traefik.Splice)
	require.NoError(t, e)
	return out
}
func TestR9EnvironmentLiteralCodec(t *testing.T) {
	for _, old := range []string{"", "OTHER=1\n", "OTHER=1", "OTHER=1\r\n", "OTHER=1\r\n# retained"} {
		for _, literal := range []string{"p$a#s s", "a\\b\"c", " # leading and trailing "} {
			t.Run(old+literal, func(t *testing.T) {
				q := []dto.ResolverVariable{{Name: "HTTPREQ_PASSWORD", Value: &literal}}
				out, values, e := resolver.Environment([]byte(old), q)
				require.NoError(t, e)
				require.True(t, strings.HasPrefix(string(out), old))
				require.Equal(t, literal, values["HTTPREQ_PASSWORD"])
				parsed, e := dotenv.ParseWithLookup(strings.NewReader(string(out)), func(string) (string, bool) { return "expanded", true })
				require.NoError(t, e)
				require.Equal(t, literal, parsed["HTTPREQ_PASSWORD"])
				if old != "" {
					require.Equal(t, strings.HasSuffix(old, "\n"), strings.HasSuffix(string(out), "\n"))
				}
			})
		}
	}
}
func TestR8R13EnvironmentRefusals(t *testing.T) {
	value := "example-new-value"
	for _, tt := range []struct {
		old  string
		vars []dto.ResolverVariable
		code dto.Reason
	}{
		{"KEY=one\nKEY=two\n", []dto.ResolverVariable{{Name: "KEY"}}, dto.EnvDuplicate},
		{"KEY=one\n", []dto.ResolverVariable{{Name: "KEY", Value: &value}}, dto.EnvKeyExists},
		{"", []dto.ResolverVariable{{Name: "KEY"}}, dto.EnvConflict},
	} {
		_, _, e := resolver.Environment([]byte(tt.old), tt.vars)
		require.Error(t, e)
		require.Equal(t, tt.code, e.(*dto.Problem).Code)
	}
	_, _, e := resolver.Environment([]byte("COMPOSE_FILE=other.yaml\n"), nil)
	require.Error(t, e)
}
func TestR10ByteExactNamesAndSpliceGolden(t *testing.T) {
	q := request()
	v := target(t, true)
	out := build(t, v, q)
	require.Contains(t, string(out.Files[0].After), "# static heading\nentryPoints:\n  web:\n    address: ':80'\n")
	require.Contains(t, string(out.Files[0].After), "# static footer")
	require.Equal(t, composeText, string(out.Files[1].After))
	q.Name = "example"
	_, e := resolver.Build(t.Context(), v, q, inventory(), traefik.Splice)
	require.Equal(t, dto.OperationConflict, e.(*dto.Problem).Code)
	flags := build(t, target(t, false), request())
	text := string(flags.Files[0].After)
	require.Contains(t, text, "image: traefik:v3.7.16 # pinned image\n")
	require.Contains(t, text, "- --entrypoints.web.address=:80 # listener\n")
	require.Contains(t, text, "  app:\n    image: example/app:1.0.0\n# operator footer\n")
	require.Contains(t, text, "--certificatesresolvers.Example.acme.storage=/certs/acme.json")
}
func TestR8LayoutVetoesBeforeEffects(t *testing.T) {
	for _, tt := range []struct{ name, content string }{
		{"anchor", strings.Replace(composeText, "image: traefik:v3.7.16", "image: &image traefik:v3.7.16", 1)},
		{"include", "include: [other.yaml]\n" + composeText},
		{"extends", strings.Replace(composeText, "    command:", "    extends: other\n    command:", 1)},
		{"scalar-command", strings.Replace(composeText, "command:\n      - --entrypoints.web.address=:80 # listener\n      - --providers.file.directory=/etc/traefik/routes", "command: --api=true", 1)},
		{"environment-static", strings.Replace(composeText, "INLINE: existing-inline-sentinel", "TRAEFIK_API: true", 1)},
		{"namespace-dependent", strings.Replace(composeText, "  app:\n", "  app:\n    network_mode: service:gateway\n", 1)},
		{"env-conflict", strings.Replace(composeText, "INLINE: existing-inline-sentinel", "HTTPREQ_ENDPOINT: http://sink.example.test", 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := target(t, false)
			v.Files[0].Before = []byte(tt.content)
			q := request()
			if tt.name == "env-conflict" {
				q.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}
				value := "http://sink.example.test"
				q.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &value}}
			}
			_, e := resolver.Build(t.Context(), v, q, inventory(), traefik.Splice)
			require.Error(t, e)
			actual, e := os.ReadFile(filepath.Join(v.Directory, v.Compose))
			require.NoError(t, e)
			require.Equal(t, composeText, string(actual))
		})
	}
}
func TestEffectiveEnvironmentMapListAndReuse(t *testing.T) {
	for _, list := range []bool{false, true} {
		for _, inject := range []bool{false, true} {
			v := target(t, false)
			v.AutoInject = inject
			if list {
				v.Files[0].Before = []byte(strings.Replace(composeText, "      INLINE: existing-inline-sentinel", "      - INLINE=existing-inline-sentinel", 1))
			}
			q := request()
			q.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}
			literal := "p$a#s s"
			endpoint := "http://sink.example.test"
			q.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &endpoint}, {Name: "HTTPREQ_PASSWORD", Value: &literal}}
			out := build(t, v, q)
			require.Contains(t, string(out.Files[0].After), "${HTTPREQ_PASSWORD}")
			require.Contains(t, string(out.Files[1].After), "HTTPREQ_PASSWORD="+"'"+literal+"'")
			for _, file := range out.Files {
				require.NoError(t, os.WriteFile(filepath.Join(v.Directory, file.Path), file.After, os.FileMode(file.Mode)))
			}
			t.Setenv("COMPOSE_FILE", "other.yaml")
			deployed, err := projects.LoadComposeProject(t.Context(), filepath.Join(v.Directory, v.Compose), v.Project, filepath.Dir(v.Directory), inject, nil, nil, nil, false, nil, nil, nil)
			require.NoError(t, err)
			require.Equal(t, []string{filepath.Join(v.Directory, v.Compose)}, deployed.ComposeFiles)
			require.Equal(t, literal, *deployed.Services[v.Service].Environment["HTTPREQ_PASSWORD"])
			require.Equal(t, endpoint, *deployed.Services[v.Service].Environment["HTTPREQ_ENDPOINT"])
			_, injected := deployed.Services["app"].Environment["REUSED"]
			require.Equal(t, inject, injected, "autoInjectEnv changes the real deployed model")
		}
	}
	v := target(t, false)
	q := request()
	q.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}
	v.Files[1].Before = []byte("HTTPREQ_ENDPOINT='http://sink.example.test'\r\n# retained")
	q.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT"}}
	out := build(t, v, q)
	require.Equal(t, v.Files[1].Before, out.Files[1].After)
}

// B2/D-A1: Docklet creates .env with FILE_PERM (0644). An untouched .env keeps
// its mode; a changed one (new secrets) is planned owner-only.
func TestEnvModeOnlyChangedEnvIsTightened(t *testing.T) {
	v := target(t, false)
	require.NoError(t, os.Chmod(filepath.Join(v.Directory, ".env"), 0644))
	for i := range v.Files {
		if v.Files[i].Path == ".env" {
			f, e := resolver.Read(v.Directory, ".env")
			require.NoError(t, e)
			f.Kind = "env"
			v.Files[i] = f
		}
	}
	envOf := func(out resolver.Target) resolver.File {
		for _, f := range out.Files {
			if f.Path == ".env" {
				return f
			}
		}
		t.Fatal("no .env")
		return resolver.File{}
	}
	unchanged := envOf(build(t, v, request()))
	require.Zero(t, unchanged.WriteMode)
	q := request()
	value := "submitted-sentinel"
	endpoint := "http://sink.example.test"
	q.Challenge = dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}
	q.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &endpoint}, {Name: "HTTPREQ_PASSWORD", Value: &value}}
	changed := envOf(build(t, v, q))
	require.EqualValues(t, 0600, changed.WriteMode)
	require.EqualValues(t, 0644, changed.Mode)
}
