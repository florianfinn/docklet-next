// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original transport-failure and graph-budget guard proofs.

package caddy

import (
	"encoding/json/v2"
	"fmt"
	"github.com/containerd/errdefs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func TestScannerQuotedWhitespaceAndBalancedPrefix(t *testing.T) {
	ds, err := scan("example.com {\n\trespond\t`a\\b # c`\r\n}\r\n")
	require.NoError(t, err)
	require.Len(t, ds, 3)
	require.Equal(t, []string{"respond", `a\b # c`}, ds[1].words)
	_, err = scan("}\nexample.com {\n")
	require.Error(t, err)
}
func TestGraphGlobalBudgets(t *testing.T) {
	for _, which := range []string{"files", "bytes"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			if which == "files" {
				for n := 0; n < hostio.MaxFiles; n++ {
					x.put(fmt.Sprintf("routes/route-%03d.caddy", n), "# fixture\n")
				}
			} else {
				root := rootText
				for n := 0; n < 17; n++ {
					p := fmt.Sprintf("extra-%02d.caddy", n)
					x.put(p, "#"+strings.Repeat("a", hostio.MaxFileBytes-2)+"\n")
					root += "import /etc/caddy/" + p + "\n"
				}
				x.put("Caddyfile", root)
			}
			_, err := readChecked(t, x.e, x.i)
			require.ErrorContains(t, err, "input-limit")
			x.noTransient()
		})
	}
}
func TestNestedMountMappingAndRootImportRefusals(t *testing.T) {
	x := fixture(t)
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "unrelated"), Destination: "/etc/caddy/routes", RW: true})
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.SourceReadOnly, inv.Eligibility.Reason)
	x.c.Mounts[1].Source = filepath.Join(x.root, "routes")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	without := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
	x.put("Caddyfile", without)
	q := x.request("create-route", baseSite())
	q.Target = "Caddyfile"
	q.Change.Kind = "insert-import"
	x.i.Source.MountTarget = "/etc/caddy/routes"
	require.False(t, x.e.rootWritable(t.Context(), x.i, x.c, "/etc/caddy/Caddyfile"))
	x.i.Source.MountTarget = "/etc/caddy"
	for _, target := range []string{"routes/app.caddy", "Caddyfile"} {
		q.Target = target
		if target == "Caddyfile" {
			x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "Caddyfile"), Destination: "/etc/caddy/Caddyfile", RW: true})
		}
		_, err := prepareChecked(t, x.e, q)
		require.Error(t, err)
	}
}
func TestPrepareReadAndValidateErrors(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	for _, p := range []string{"/absolute/staging", "routes/ordinary"} {
		i.StagingPath = p
		require.ErrorContains(t, validateChecked(t, x.e, i), "invalid-input")
	}
	i.StagingPath = "routes/.docklet-staging-example"
	x.put("routes/extra.secret.caddy", "secret payload")
	require.ErrorContains(t, validateChecked(t, x.e, i), "opaque-input")
	r, err := readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.NotEmpty(t, r.OpaqueFiles)
	for _, f := range r.Files {
		require.NotContains(t, f.Content, "secret payload")
	}
	require.NoError(t, os.Remove(filepath.Join(x.root, "routes/extra.secret.caddy")))
	x.i.RootFile = "/etc/caddy/other"
	i.Request.Instance = x.i
	require.Error(t, validateChecked(t, x.e, i))
	x.i.RootFile = "/etc/caddy/Caddyfile"
	x.put("routes/foreign.caddy", "foreign.example.net {\n respond ok\n}\n")
	r, err = readChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Equal(t, "text-only", r.Routes[len(r.Routes)-1].Class)
	x.c.ID = "example-recreated"
	_, err = readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "container-changed")
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "container-changed")
	require.Error(t, validateChecked(t, x.e, i))
}
func TestResumeRefusesBadAdminAndAdaptJSON(t *testing.T) {
	for _, which := range []string{"adapt-fails", "adapt-json", "adapt-null", "admin-fails", "resume-admin-off"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
			x.execFn = func(args []string) hostio.ExecResult {
				if args[0] == "wget" {
					if which == "admin-fails" {
						return hostio.ExecResult{ExitCode: 1}
					}
					return httpResult(200, "{}")
				}
				if args[1] == "adapt" {
					switch which {
					case "adapt-fails":
						return hostio.ExecResult{ExitCode: 1, Stderr: []byte("adapt rejected")}
					case "adapt-json":
						return hostio.ExecResult{Stdout: []byte("bad")}
					case "adapt-null":
						return hostio.ExecResult{Stdout: []byte("null")}
					}
				}
				return hostio.ExecResult{}
			}
			if which == "resume-admin-off" {
				x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
			}
			q := x.request("create-route", baseSite())
			q.Acknowledgements.Restart = true
			_, err := prepareChecked(t, x.e, q)
			require.Error(t, err)
		})
	}
}
func TestReadbackRejectsMissingHostPathAndWrongHandler(t *testing.T) {
	s := baseSite()
	s.Path = "/api"
	for _, pair := range [][2]string{{`"host":["app.example.com"]`, `"host":["other.example.net"]`}, {`"path":["/api*"]`, `"path":["/other*"]`}, {`"handler":"reverse_proxy"`, `"handler":"static_response"`}, {`"upstreams":[{"dial":"192.0.2.80:8080"}]`, `"upstreams":[]`}} {
		var config map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.Replace(configFor(s), pair[0], pair[1], 1)), &config))
		require.False(t, containsSite(config, s, true))
	}
	var safe bool
	require.NotPanics(t, func() { safe = upstreamsEqual(map[string]any{}, []string{"http://["}) })
	require.False(t, safe)
	// OR branches preserve each complete matcher; an unrelated branch cannot
	// supply the missing host of another branch.
	raw := `{"apps":{"http":{"servers":{"example":{"routes":[{"match":[{"host":["other.example.net"],"path":["/api*"]},{"host":["app.example.com"],"path":["/other*"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"192.0.2.80:8080"}]}]}]}}}}}`
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &config))
	require.False(t, containsSite(config, s, true))
}
func TestIntentStateDecoderAndNoCaddyMarker(t *testing.T) {
	x := fixture(t)
	i := proxy.Intent{Engine: []byte("bad")}
	_, err := x.e.Observe(t.Context(), i, false)
	require.Error(t, err)
	require.Error(t, x.e.Reload(t.Context(), i))
	require.Error(t, x.e.Mark(t.Context(), i))
}

func TestInventoryImportLocationAndDiagnostics(t *testing.T) {
	x := fixture(t)
	x.i.RootFile = ""
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	x.i.RootFile = "/etc/caddy/Caddyfile"
	for _, dir := range []string{"", "routes\n"} {
		i := x.i
		i.RouteDirectory = dir
		_, err := readChecked(t, x.e, i)
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		require.Equal(t, dto.InvalidInput, p.Code)
	}
	for _, which := range []string{"nested", "other-import"} {
		if which == "nested" {
			x.put("Caddyfile", "legacy.example.net {\n import /etc/caddy/routes/*.caddy\n}\n")
		} else {
			x.put("other.caddy", "other.example.net {\n respond ok\n}\n")
			x.put("Caddyfile", "import /etc/caddy/other.caddy\n")
		}
		inv, err = inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.ImportPresent)
		require.Equal(t, dto.ImportMissing, inv.Eligibility.Reason)
		q := x.request("create-route", baseSite())
		_, err = prepareChecked(t, x.e, q)
		require.ErrorContains(t, err, "import-missing")
	}
	x.put("Caddyfile", rootText+"import /etc/caddy/{$FILE}\n")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.Equal(t, dto.IncompleteInventory, inv.Eligibility.Reason)
	x.put("extra/hidden.secret.caddy", "opaque payload")
	x.put("Caddyfile", rootText+"import /etc/caddy/extra/*.caddy\n")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.OpaqueInput, inv.Eligibility.Reason)
}
func TestReadErrorDiagnosticsAtTheRealDockerSeam(t *testing.T) {
	x := fixture(t)
	for _, text := range []string{"example.com {\n", rootText + "import /etc/caddy/missing.caddy\n"} {
		x.put("Caddyfile", text)
		_, err := readChecked(t, x.e, x.i)
		require.Error(t, err)
		if text == "example.com {\n" {
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "/etc/caddy/Caddyfile", p.Path)
			require.Equal(t, dto.InvalidGraph, p.Code)
		} else {
			require.True(t, errdefs.IsNotFound(err))
		}
	}
	require.NoError(t, os.Remove(filepath.Join(x.root, "Caddyfile")))
	_, err := readChecked(t, x.e, x.i)
	require.True(t, errdefs.IsNotFound(err))
	x.put("Caddyfile", rootText)
	x.c.Config.Cmd = []string{"run", "--adapter", "json"}
	_, err = readChecked(t, x.e, x.i)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.UnsupportedFormat, p.Code)
}

func TestCopyFailureIsNotMisreportedAsMissingImport(t *testing.T) {
	x := fixture(t)
	failed := false
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if !failed && r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/archive") && r.URL.Query().Get("path") == "/etc/caddy/routes" {
			failed = true
			http.Error(w, `{"message":"temporary failure"}`, 500)
			return true
		}
		return false
	}
	_, err := readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "temporary failure")
}
func TestOrphanedFilesAndMissingRouteDirectoryStayReadOnly(t *testing.T) {
	x := fixture(t)
	without := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
	x.put("Caddyfile", without)
	require.NoError(t, os.Remove(filepath.Join(x.root, "routes")))
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	x.put("routes/extra.secret.caddy", "opaque payload")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.Equal(t, dto.OpaqueInput, inv.Eligibility.Reason)
	require.NoError(t, os.Remove(filepath.Join(x.root, "routes/extra.secret.caddy")))
	x.put("routes/bad.caddy", "example.com {\n")
	_, err = readChecked(t, x.e, x.i)
	require.ErrorContains(t, err, "invalid-graph")
}
func TestGlobalOptionsIgnoreUnusedSnippetsAndSiteBlocks(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", rootText+"(unused) {\n admin off\n persist_config off\n}\n")
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.AdminOff)
	require.True(t, inv.PersistConfig)
	x.put("Caddyfile", "{\n admin 127.0.0.1:2020\n persist_config on\n}\nimport /etc/caddy/routes/*.caddy\n")
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.AdminOff)
	require.Equal(t, "127.0.0.1:2020", inv.AdminAddress)
	require.True(t, inv.PersistConfig)
}
func TestQuotedImportTokensAndMissingCommandValues(t *testing.T) {
	require.Equal(t, `"/etc/caddy/my routes/*.caddy"`, importToken("/etc/caddy/my routes/*.caddy"))
	for _, args := range [][]string{{"run", "--config"}, {"run", "--adapter"}} {
		_, _, err := command(container.InspectResponse{Config: &container.Config{Cmd: args}})
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		require.Equal(t, "missing command value", p.Detail)
	}
	_, _, err := command(container.InspectResponse{})
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "missing container command", p.Detail)
}
