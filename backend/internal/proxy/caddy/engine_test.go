// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original break-map proofs through the real DockerRuntime and engine.

package caddy

import (
	"encoding/json"
	"fmt"
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

func configFor(s Site) string {
	match := map[string]any{"host": []string{s.Domain}}
	handler := map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": strings.TrimPrefix(s.Upstreams[0], "http://")}}}
	route := map[string]any{"match": []any{match}, "handle": []any{handler}}
	if s.Path != "" {
		match["path"] = []string{s.Path + "*"}
	}
	raw, _ := json.Marshal(map[string]any{"apps": map[string]any{"http": map[string]any{"servers": map[string]any{"example": map[string]any{"routes": []any{route}}}}}})
	return string(raw)
}
func (x *dockerFixture) serving() {
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] != "wget" {
			if args[1] == "adapt" {
				return hostio.ExecResult{Stdout: []byte("{}")}
			}
			return hostio.ExecResult{}
		}
		b, err := os.ReadFile(filepath.Join(x.root, "routes/app.caddy"))
		s, parseErr := ParseSite(string(b))
		if strings.Contains(args[len(args)-1], "/config/") {
			if err != nil || parseErr != nil {
				return httpResult(200, "{}")
			}
			return httpResult(200, configFor(s))
		}
		if err != nil {
			return httpResult(404, "")
		}
		return httpResult(200, strings.Join(s.Upstreams, ","))
	}
}
func TestP15OwnedRouteCRUDPreservesEveryOtherByte(t *testing.T) {
	x := fixture(t)
	x.serving()
	s := baseSite()
	q := x.request("create-route", s)
	o, err := applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
	text, _ := renderChecked(t, s)
	require.Equal(t, text, x.bytes(q.Target))
	require.Equal(t, rootText, x.bytes("Caddyfile"))
	x.noTransient()
	x.put("routes/foreign.caddy", "# untouched sibling\nother.example.net {\n respond ok\n}\n")
	s.Upstreams = []string{"http://192.0.2.81:8080"}
	q = x.request("edit-route", s)
	// Non-canonical text (leading blank line, tab indentation, trailing blank lines) is
	// written exactly as submitted.
	q.Change.Advanced = "\n" + strings.Replace(q.Change.Advanced, "  route {", "\t route {", 1) + "\n\n"
	o, err = applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, q.Change.Advanced, x.bytes(q.Target))
	require.Equal(t, rootText, x.bytes("Caddyfile"))
	require.Equal(t, "# untouched sibling\nother.example.net {\n respond ok\n}\n", x.bytes("routes/foreign.caddy"))
	x.noTransient()
	q = x.request("delete-route", s)
	o, err = applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
	_, err = os.Stat(filepath.Join(x.root, q.Target))
	require.True(t, os.IsNotExist(err))
	x.noTransient()
}
func TestP1P2P23StagedGraphRejectsInvalidCandidateAndSibling(t *testing.T) {
	for _, which := range []string{"sibling", "candidate"} {
		t.Run(which, func(t *testing.T) {
			x := fixture(t)
			s := baseSite()
			if which == "sibling" {
				x.put("routes/foreign.caddy", "other.example.net {\n provisioning_invalid\n}\n")
			} else {
				s.TLS = "auto"
				s.Provider = "unavailable_dns"
				s.Credential = "DNS_API_TOKEN"
			}
			x.execFn = func(args []string) hostio.ExecResult {
				if args[0] == "wget" {
					return httpResult(404, "")
				}
				if args[1] == "validate" {
					abs := args[3]
					if abs == "/etc/caddy/Caddyfile" {
						return hostio.ExecResult{}
					}
					require.Contains(t, abs, ".docklet-staging-")
					root := x.bytes(strings.TrimPrefix(abs, "/etc/caddy/"))
					require.NotContains(t, root, "import /etc/caddy/routes/*.caddy")
					for _, line := range strings.Split(root, "\n") {
						if strings.HasPrefix(line, "import ") {
							p := strings.TrimPrefix(line, "import /etc/caddy/")
							text := x.bytes(p)
							if strings.Contains(text, "provisioning_invalid") || strings.Contains(text, "unavailable_dns") {
								return hostio.ExecResult{ExitCode: 1, Stderr: []byte("Caddyfile:2: provisioning failed: module not registered")}
							}
						}
					}
				}
				return hostio.ExecResult{}
			}
			q := x.request("create-route", s)
			o, err := applyChecked(t, x.a, q)
			require.NoError(t, err)
			require.Equal(t, dto.Rejected, o.State)
			require.Equal(t, dto.InvalidGraph, o.Reason)
			require.Equal(t, rootText, x.bytes("Caddyfile"))
			_, err = os.Stat(filepath.Join(x.root, q.Target))
			require.True(t, os.IsNotExist(err))
			x.noTransient()
			res, err := x.d.Exec(t.Context(), x.i, "validate", "/etc/caddy/Caddyfile", "", "")
			require.NoError(t, err)
			require.Zero(t, res.ExitCode, "old graph still passes the daemon validator")
		})
	}
}
func TestP2CommandExitIsIndependentFromTransport(t *testing.T) {
	res := hostio.ExecResult{ExitCode: 1, Stderr: []byte("dns provider not registered")}
	err := commandError(res, nil, "Caddyfile")
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.InvalidGraph, p.Code)
	require.Contains(t, p.Detail, "dns provider not registered")
	require.ErrorContains(t, commandError(hostio.ExecResult{}, dto.Fail(dto.ExecRunning, 409), ""), "exec-running")
}
func TestP9ResumeNormalizesJSONAndRequiresDivergenceAcknowledgement(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	running := "{\"apps\":{},\"admin\":{}}"
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "wget" {
			if strings.Contains(args[len(args)-1], "/config/") {
				return httpResult(200, running)
			}
			return httpResult(404, "")
		}
		if args[1] == "adapt" {
			return hostio.ExecResult{Stdout: []byte("{ \"admin\": {}, \"apps\": {} }")}
		}
		return hostio.ExecResult{}
	}
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.Resume)
	require.False(t, inv.RunningDiverged)
	running = "{\"apps\":{\"http\":{}}}"
	inv, err = inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.True(t, inv.RunningDiverged)
	q := x.request("create-route", baseSite())
	_, err = prepareChecked(t, x.e, q)
	require.ErrorContains(t, err, "acknowledgement-required")
	x.noTransient()
	q.Acknowledgements.ReplaceRunningConfig = true
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	require.True(t, i.Reload)
	require.False(t, i.Restart)
	require.NoError(t, x.e.Reload(t.Context(), i))
	require.Equal(t, []string{"caddy", "reload", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"}, x.commands[len(x.commands)-1])
}
func TestP10AdminOffRefusesBeforeWritesAndRestoresFailedRestart(t *testing.T) {
	for _, secondFails := range []bool{false, true} {
		t.Run(fmt.Sprint(secondFails), func(t *testing.T) {
			x := fixture(t)
			x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
			x.serving()
			q := x.request("create-route", baseSite())
			o, err := applyChecked(t, x.a, q)
			require.NoError(t, err)
			require.Equal(t, dto.Rejected, o.State)
			require.Equal(t, dto.AcknowledgementRequired, o.Reason)
			require.Zero(t, x.restarts)
			x.noTransient()
			q.ID = "restart-acknowledged"
			q.Acknowledgements.Restart = true
			x.restartFn = func() {
				x.c.State.Running = x.restarts == 2 && !secondFails
				x.c.State.StartedAt = fmt.Sprintf("2026-11-16T%02d:00:00Z", x.restarts+1)
			}
			o, err = applyChecked(t, x.a, q)
			require.NoError(t, err)
			require.Equal(t, 2, x.restarts)
			if secondFails {
				require.Equal(t, dto.RecoveryRequired, o.State)
			} else {
				require.Equal(t, dto.Restored, o.State)
			}
			_, err = os.Stat(filepath.Join(x.root, q.Target))
			require.True(t, os.IsNotExist(err))
			x.noTransient()
			for _, args := range x.commands {
				if args[0] == "caddy" {
					require.NotEqual(t, "reload", args[1])
				}
			}
		})
	}
}
func TestP22P23EligibilityFromRealCommandMountsAndSecretArchive(t *testing.T) {
	cases := []struct {
		name   string
		change func(*dockerFixture)
		reason dto.Reason
	}{
		{"persistence", func(x *dockerFixture) {
			x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
			x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "persist_config off", 1))
		}, dto.PersistenceOff},
		{"resume-admin-off", func(x *dockerFixture) {
			x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
			x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
		}, dto.AdminUnavailable},
		{"relative-import", func(x *dockerFixture) { x.put("Caddyfile", rootText+"import snippets/extra.caddy\n") }, dto.RelativeImport},
		{"secret", func(x *dockerFixture) { x.put("routes/hidden.SECRET.caddy", "secret payload") }, dto.OpaqueInput},
		{"missing-directory", func(x *dockerFixture) { require.NoError(t, os.Remove(filepath.Join(x.root, "routes"))) }, dto.ImportMissing},
		{"single-file-route", func(x *dockerFixture) {
			x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "Caddyfile"), Destination: "/etc/caddy/routes", RW: true})
		}, dto.SingleFileMount},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			x := fixture(t)
			tt.change(x)
			inv, err := inventoryChecked(t, x.e, x.i)
			require.NoError(t, err)
			require.False(t, inv.Eligibility.Writable)
			require.Equal(t, tt.reason, inv.Eligibility.Reason)
			r := dto.ApplyRequest{Instance: x.i, Target: "routes/app.caddy", Change: dto.Change{Kind: "create-route"}}
			_, err = prepareChecked(t, x.e, r)
			require.Error(t, err)
			require.Empty(t, x.commands)
			x.noTransient()
		})
	}
}
func TestP28ImportInsertionAndReinsertionOnlyAppendsOneTopLevelLine(t *testing.T) {
	x := fixture(t)
	without := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
	x.put("Caddyfile", without)
	for n := 0; n < 2; n++ {
		inv, err := inventoryChecked(t, x.e, x.i)
		require.NoError(t, err)
		require.False(t, inv.ImportPresent)
		require.True(t, inv.Eligibility.Writable)
		require.Equal(t, "import /etc/caddy/routes/*.caddy", inv.ImportLine)
		g, err := x.d.Graph(t.Context(), x.i)
		require.NoError(t, err)
		q := dto.ApplyRequest{ID: fmt.Sprintf("insert-%d", n), Instance: x.i, Generation: x.a.Generation(), Registration: x.i.Registration, Target: "Caddyfile", Expected: proxy.Snapshot([]byte(without), true).FileState, GraphHashes: g, Change: dto.Change{Kind: "insert-import"}}
		o, err := applyChecked(t, x.a, q)
		require.NoError(t, err)
		require.Equal(t, dto.Verified, o.State)
		require.Equal(t, rootText, x.bytes("Caddyfile"))
		x.noTransient()
		x.put("Caddyfile", without)
	}
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: filepath.Join(x.root, "Caddyfile"), Destination: "/etc/caddy/Caddyfile", RW: true})
	inv, err := inventoryChecked(t, x.e, x.i)
	require.NoError(t, err)
	require.False(t, inv.Eligibility.Writable)
	require.False(t, inv.ImportPresent)
	require.NotEmpty(t, inv.ImportLine)
}
func TestP6P7DelayedEvidenceAndFirstResponse(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	st := state{Root: "/etc/caddy/Caddyfile", AdminOff: true, Before: hostio.Response{Status: 404, Headers: map[string]string{"server": "Caddy"}}, NewSite: &s}
	data, _ := json.Marshal(st)
	i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: "create-route"}}, Engine: data, PublishedAt: x.now}
	calls := 0
	x.execFn = func(args []string) hostio.ExecResult {
		calls++
		if x.waits >= 3 {
			return httpResult(401, "auth")
		}
		return httpResult(404, "")
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
	require.Equal(t, 3, x.waits)
	require.Equal(t, 4, calls)
	require.NotEmpty(t, x.since)
	require.Equal(t, fmt.Sprintf("%d.000000000", i.PublishedAt.Unix()), x.since[0])
	x.waits = 0
	x.execFn = func(args []string) hostio.ExecResult { return httpResult(404, "") }
	o, err = x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.NoObservableDifference, o.Reason)
	require.Equal(t, 50, x.waits)
	for _, code := range []int{502, 503, 504} {
		require.False(t, routing(st.Before, hostio.Response{Status: code}, "create-route"))
	}
	redirected := httpResult(302, "later-body")
	redirected.Stderr = []byte("  HTTP/1.1 404 Not Found\n  Server: Caddy\n\n  HTTP/1.1 200 OK\n\n")
	x.execFn = func(args []string) hostio.ExecResult { return redirected }
	x.waits = 0
	o, err = x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
}
func TestObserveConfigurationEvidenceAndEpochChange(t *testing.T) {
	x := fixture(t)
	s := baseSite()
	s.TLS = "auto"
	st := state{Root: "/etc/caddy/Caddyfile", Admin: "localhost:2019", NewSite: &s}
	data, _ := json.Marshal(st)
	i := proxy.Intent{Request: dto.ApplyRequest{Instance: x.i, Change: dto.Change{Kind: "create-route"}}, Engine: data, PublishedAt: x.now}
	x.execFn = func(args []string) hostio.ExecResult {
		if strings.Contains(args[len(args)-1], "/config/") {
			return httpResult(200, configFor(s))
		}
		return hostio.ExecResult{ExitCode: 1}
	}
	o, err := x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Configuration, o.Evidence)
	x.execFn = func(args []string) hostio.ExecResult {
		x.c.State.StartedAt = nextEpoch
		return httpResult(200, configFor(s))
	}
	o, err = x.e.Observe(t.Context(), i, false)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.ContainerChanged, o.Reason)
}

func TestP10AcknowledgedRestartVerifiesNewEpoch(t *testing.T) {
	x := fixture(t)
	x.put("Caddyfile", strings.Replace(rootText, "admin localhost:2019", "admin off", 1))
	x.serving()
	q := x.request("create-route", baseSite())
	q.Acknowledgements.Restart = true
	o, err := applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, dto.Routing, o.Evidence)
	require.Equal(t, 1, x.restarts)
	require.Equal(t, nextEpoch, x.c.State.StartedAt)
	x.noTransient()
}
func TestP28FailedReloadRestoresExactRoot(t *testing.T) {
	x := fixture(t)
	without := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
	x.put("Caddyfile", without)
	g, err := x.d.Graph(t.Context(), x.i)
	require.NoError(t, err)
	q := dto.ApplyRequest{ID: "insert-fails-reload", Instance: x.i, Generation: x.a.Generation(), Registration: x.i.Registration, Target: "Caddyfile", Expected: proxy.Snapshot([]byte(without), true).FileState, GraphHashes: g, Change: dto.Change{Kind: "insert-import"}}
	reloads := 0
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "caddy" && args[1] == "reload" {
			reloads++
			if reloads == 1 {
				return hostio.ExecResult{ExitCode: 1, Stderr: []byte("reload refused")}
			}
		}
		return hostio.ExecResult{}
	}
	o, err := applyChecked(t, x.a, q)
	require.NoError(t, err)
	require.Equal(t, dto.Restored, o.State)
	require.Equal(t, 2, reloads)
	require.Equal(t, without, x.bytes("Caddyfile"))
	x.noTransient()
}
func TestP23CrashAfterStagingRemovesCandidateGraph(t *testing.T) {
	x := fixture(t)
	q := x.request("create-route", baseSite())
	i, err := prepareChecked(t, x.e, q)
	require.NoError(t, err)
	i.TempPath = "routes/.docklet-" + q.ID + ".tmp"
	i.StagingPath = "routes/.docklet-staging-" + q.ID
	require.NoError(t, x.d.Stage(t.Context(), i))
	require.NoError(t, validateChecked(t, x.e, i))
	digest, err := proxy.Digest(q)
	require.NoError(t, err)
	intent, err := json.Marshal(i)
	require.NoError(t, err)
	op := dto.Operation{ID: q.ID, InstanceID: x.i.ID, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: dto.Pending, Phase: dto.Staging, Target: q.Target, Old: i.Old.FileState, New: i.New.FileState}
	result, err := json.Marshal(op)
	require.NoError(t, err)
	require.NoError(t, x.d.DB.Create(&proxy.Journal{ID: q.ID, Source: "host\x00" + x.i.Source.Canonical, Generation: q.Generation, Digest: digest, Registration: q.Registration, State: op.State, Phase: op.Phase, Intent: intent, Result: result}).Error)
	restarted, err := proxy.NewAgent(x.d.DB.DB, x.d, proxy.Engines{dto.Caddy: x.e})
	require.NoError(t, err)
	require.NoError(t, restarted.Settle(t.Context()))
	op, err = restarted.Query(t.Context(), dto.Query{ID: q.ID, Instance: x.i, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, err)
	require.Equal(t, dto.Rejected, op.State)
	x.noTransient()
}
