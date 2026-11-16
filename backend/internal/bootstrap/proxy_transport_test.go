// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original protocol evidence through actual Direct and Edge senders and receivers.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	p "github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type proxyWireRuntime struct {
	mu     sync.Mutex
	target p.Bytes
	// inside/resume hold Validate so a test can drop the connection after admission.
	inside, resume chan struct{}
	validateErr    error
}

func (f *proxyWireRuntime) Rebind(_ context.Context, i dto.Instance) (dto.Instance, error) {
	return i, nil
}
func (f *proxyWireRuntime) Check(context.Context, dto.Instance) error { return nil }
func (f *proxyWireRuntime) Target(context.Context, dto.Instance, string) (p.Bytes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.target, nil
}
func (f *proxyWireRuntime) Graph(context.Context, dto.Instance) (map[string]string, error) {
	return map[string]string{}, nil
}
func (f *proxyWireRuntime) Stage(context.Context, p.Intent) error { return nil }
func (f *proxyWireRuntime) Publish(_ context.Context, i p.Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.target = i.New
	return nil
}
func (f *proxyWireRuntime) Restore(_ context.Context, i p.Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.target = i.Old
	return nil
}
func (f *proxyWireRuntime) Cleanup(context.Context, p.Intent) error { return nil }
func (f *proxyWireRuntime) Epoch(context.Context, dto.Instance) (bool, string, error) {
	return true, "example-epoch", nil
}

type proxyWireEngine struct{ f *proxyWireRuntime }

func (e *proxyWireEngine) Inventory(context.Context, dto.Instance) (dto.Inventory, error) {
	return dto.Inventory{Eligibility: dto.Eligibility{Writable: true}}, nil
}
func (e *proxyWireEngine) Read(context.Context, dto.Instance) (dto.ReadResult, error) {
	return dto.ReadResult{}, nil
}
func (e *proxyWireEngine) Prepare(ctx context.Context, q dto.ApplyRequest) (p.Intent, error) {
	b, _ := e.f.Target(ctx, q.Instance, q.Target)
	return p.Intent{Old: b, New: p.Snapshot([]byte("new"), true), GraphHashes: map[string]string{}, Mode: 0644}, nil
}
func (e *proxyWireEngine) Validate(context.Context, p.Intent) error {
	if e.f.inside != nil {
		close(e.f.inside)
		<-e.f.resume
	}
	return e.f.validateErr
}
func (e *proxyWireEngine) Reload(context.Context, p.Intent) error  { return nil }
func (e *proxyWireEngine) Restart(context.Context, p.Intent) error { return nil }
func (e *proxyWireEngine) Observe(context.Context, p.Intent, bool) (p.Observation, error) {
	return p.Observation{State: dto.Verified, Evidence: dto.Routing}, nil
}
func (e *proxyWireEngine) Mark(context.Context, p.Intent) error    { return nil }
func (e *proxyWireEngine) Cleanup(context.Context, p.Intent) error { return nil }
func proxyWireFixture(t *testing.T, mode string) (*m27ModuleFixture, *p.Agent, *p.HostTransport, dto.ApplyRequest) {
	t.Helper()
	var a *p.Agent
	x := newM27ModuleFixture(t, mode, false, func(x *m27ModuleFixture, api huma.API) {
		db := m27DB(t)
		f := &proxyWireRuntime{target: p.Snapshot([]byte("old"), true)}
		var err error
		a, err = p.NewAgent(db.DB, f, p.Engines{dto.Traefik: &proxyWireEngine{f}})
		require.NoError(t, err)
		m := &p.Module{Agent: a, Settings: x.agentSettings, Modules: x.agentManager}
		m.RegisterRoutes(api, true)
	})
	require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	tr := &p.HostTransport{Hosts: x.hosts}
	reg, err := tr.Registration(t.Context(), "example")
	require.NoError(t, err)
	q := dto.ApplyRequest{ID: "example-operation", Generation: a.Generation(), Registration: reg, Target: "route.yml", Expected: p.Snapshot([]byte("old"), true).FileState, GraphHashes: map[string]string{}, Change: dto.Change{Kind: "edit-route"}, Instance: dto.Instance{ID: "example-instance", Authority: dto.FileAuthority, EnvironmentID: "example", Kind: dto.Traefik, Generation: a.Generation(), Registration: reg, Identity: dto.Identity{ContainerName: "example-proxy", ContainerID: "example-container", StartedAt: "example-epoch"}, Source: dto.Source{Kind: dto.ProjectSource, Canonical: "/example/projects/proxy", MountTarget: "/etc/proxy"}, Eligibility: dto.Eligibility{Writable: true}}}
	return x, a, tr, q
}
func TestM27P11RealDirectEdgeOneDispatch(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, boundary := range []string{"lost-before-admission", "lost-after-effect", "replaced-journal", "changed-registration"} {
			t.Run(mode+"/"+boundary, func(t *testing.T) {
				x, a, tr, q := proxyWireFixture(t, mode)
				calls := 0
				x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
					if strings.HasSuffix(r.URL.Path, "/apply") {
						calls++
						if boundary != "lost-before-admission" {
							next.ServeHTTP(httptest.NewRecorder(), r)
						}
						if boundary == "replaced-journal" {
							require.NoError(t, a.DB.Exec("UPDATE proxy_generation SET generation = ? WHERE id = 1", "replacement-generation").Error)
							require.NoError(t, a.ReloadGeneration(t.Context()))
						}
						if boundary == "changed-registration" {
							require.NoError(t, x.f.db.Exec("UPDATE environments SET credential_revision = credential_revision + 1 WHERE id = ?", "example").Error)
						}
						w.WriteHeader(503)
						return
					}
					next.ServeHTTP(w, r)
				}
				h := &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}
				o, err := h.Apply(t.Context(), q)
				require.NoError(t, err)
				want := dto.RecoveryRequired
				if boundary == "lost-before-admission" {
					want = dto.NotApplied
				}
				if boundary == "lost-after-effect" {
					want = dto.Verified
				}
				require.Equal(t, want, o.State)
				require.Equal(t, 1, calls)
				if boundary != "changed-registration" {
					again, err := h.Query(t.Context(), q.ID)
					require.NoError(t, err)
					require.Equal(t, want, again.State)
				}
				require.Equal(t, 1, calls, "query and fence must not resend Apply")
			})
		}
	}
}
func TestM27P19OutcomeFixturesRealDirectEdge(t *testing.T) {
	type entry struct {
		Transport string          `json:"transport"`
		State     dto.State       `json:"state"`
		Reason    dto.Reason      `json:"reason"`
		Body      json.RawMessage `json:"body"`
	}
	entries := []entry{}
	codes := []dto.Reason{dto.NoObservableDifference, dto.NotObservable, dto.ContainerChanged, dto.LogsUnavailable, dto.GenerationMismatch, dto.OperationConflict, dto.ModuleBusy, dto.StaleHash, dto.SourceReadOnly, dto.UnknownOperation, dto.Fenced, dto.RegistrationChanged, dto.JournalLost, dto.InvalidInput, dto.InputLimit, dto.ExecFailed, dto.ExecRunning, dto.ProxyRejected, dto.RestoreFailed, dto.InvalidGraph, dto.IncompleteInventory, dto.OpaqueInput, dto.SingleFileMount, dto.RefusedBind, dto.ReadOnlyMount, dto.UnknownSource, dto.UnsupportedMetadata, dto.WatchOff, dto.UnsupportedFormat, dto.TemplateInput, dto.RuleSyntaxV2, dto.SharedAcrossFiles, dto.PersistenceOff, dto.RelativeImport, dto.ImportMissing, dto.AcknowledgementRequired, dto.ProbeUnavailable, dto.ProbeOptionUnavailable, dto.AdminUnavailable, dto.AgentUnavailable, dto.EngineUnavailable, dto.TargetChanged}
	for _, mode := range []string{"direct", "edge"} {
		x, a, tr, q := proxyWireFixture(t, mode)
		o, err := tr.Apply(t.Context(), q)
		require.NoError(t, err)
		o.Generation = "example-generation"
		o.Registration = "example-registration"
		o.Digest = strings.Repeat("a", 64)
		o.CreatedAt = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
		o.FinishedAt = nil
		for _, state := range []dto.State{dto.Pending, dto.Verified, dto.AppliedUnverified, dto.Rejected, dto.Restored, dto.RecoveryRequired, dto.Released, dto.NotApplied} {
			for _, reason := range append([]dto.Reason{""}, codes...) {
				o.State = state
				o.Reason = reason
				raw, err := json.Marshal(o)
				require.NoError(t, err)
				require.NoError(t, a.DB.Exec("UPDATE proxy_journal SET state = ?, result = ? WHERE id = ?", state, raw, q.ID).Error)
				// Avoid settlement changing fixture pending states. This test is a wire
				// projection, not another executor run; the actual query handler is replaced
				// at the HTTP response boundary while the actual sender/tunnel remains real.
				x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
					if strings.Contains(r.URL.Path, "/operations/") {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": o})
						return
					}
					next.ServeHTTP(w, r)
				}
				got, err := tr.Query(t.Context(), dto.Query{ID: q.ID, Generation: q.Generation, Registration: q.Registration, Instance: q.Instance})
				require.NoError(t, err)
				require.Equal(t, state, got.State)
				require.Equal(t, reason, got.Reason)
				b, err := json.Marshal(map[string]any{"success": true, "data": got})
				require.NoError(t, err)
				entries = append(entries, entry{mode, state, reason, b})
			}
		}
	}
	for _, mode := range []string{"direct", "edge"} {
		x, _, tr, q := proxyWireFixture(t, mode)
		o, err := tr.Apply(t.Context(), q)
		require.NoError(t, err)
		o.Generation, o.Registration, o.Digest = "example-generation", "example-registration", strings.Repeat("a", 64)
		o.CreatedAt, o.FinishedAt = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), nil
		o.State, o.Reason = dto.Rejected, dto.InvalidGraph
		o.Diagnostic = &dto.Diagnostic{Path: "routes/app.caddy", Line: 3, Detail: "Error: adapting config: unrecognized directive: example_directive"}
		x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
			if strings.Contains(r.URL.Path, "/operations/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": o})
				return
			}
			next.ServeHTTP(w, r)
		}
		got, err := tr.Query(t.Context(), dto.Query{ID: q.ID, Generation: q.Generation, Registration: q.Registration, Instance: q.Instance})
		require.NoError(t, err)
		require.Equal(t, o.Diagnostic, got.Diagnostic)
		b, err := json.Marshal(map[string]any{"success": true, "data": got})
		require.NoError(t, err)
		entries = append(entries, entry{mode, got.State, got.Reason, b})
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		b, err := json.Marshal(entry)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	raw := []byte("[\n" + strings.Join(lines, ",\n") + "\n]\n")
	fixture := filepath.Join("..", "..", "..", "types", "proxy", "testdata", "outcomes.json")
	if os.Getenv("UPDATE_PROXY_OUTCOMES") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(fixture), 0755))
		require.NoError(t, os.WriteFile(fixture, raw, 0644))
	}
	want, err := os.ReadFile(fixture)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(raw))
}

func TestM27ProxyRefreshesBindingBeforeNewOperation(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x, a, tr, q := proxyWireFixture(t, mode)
			stale := q.Instance
			stale.Generation = "previous-generation"
			stale.Registration = "previous-registration"
			raw, err := json.Marshal(stale)
			require.NoError(t, err)
			require.NoError(t, x.f.db.Exec("INSERT INTO proxy_instances(id,environment_id,data) VALUES (?,?,?)", stale.ID, stale.EnvironmentID, raw).Error)
			m := &p.Module{Hub: &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}, Modules: x.manager, Transport: tr}
			got, err := m.CreateOperation(t.Context(), stale.ID, dto.OperationRequest{Target: q.Target, Expected: q.Expected, GraphHashes: q.GraphHashes, Change: q.Change})
			require.NoError(t, err)
			require.Equal(t, dto.Verified, got.State)
			require.Equal(t, a.Generation(), got.Generation)
			require.Equal(t, q.Registration, got.Registration)
			require.NotEqual(t, q.ID, got.ID)
		})
	}
}

// Candidate run F4: after a proxy restart (same container, new StartedAt) a new operation
// fences the current epoch from the inventory, not the registration's stale one.
func TestM27ProxyNewOperationFencesTheCurrentEpoch(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x, _, tr, q := proxyWireFixture(t, mode)
			stale := q.Instance
			stale.Identity.StartedAt = "2026-11-14T00:00:00Z"
			require.NotEqual(t, q.Instance.Identity.StartedAt, stale.Identity.StartedAt)
			raw, err := json.Marshal(stale)
			require.NoError(t, err)
			require.NoError(t, x.f.db.Exec("INSERT INTO proxy_instances(id,environment_id,data) VALUES (?,?,?)", stale.ID, stale.EnvironmentID, raw).Error)
			m := &p.Module{Hub: &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}, Modules: x.manager, Transport: tr}
			got, err := m.CreateOperation(t.Context(), stale.ID, dto.OperationRequest{Target: q.Target, Expected: q.Expected, GraphHashes: q.GraphHashes, Change: q.Change})
			require.NoError(t, err)
			require.Equal(t, dto.Verified, got.State)
			saved, err := m.Instance(t.Context(), stale.ID)
			require.NoError(t, err)
			require.Equal(t, q.Instance.Identity.StartedAt, saved.Identity.StartedAt)
		})
	}
}

func TestM27ProxyAdmissionRaceUsesHubUnknownEnvelope(t *testing.T) {
	x, a, tr, q := proxyWireFixture(t, "direct")
	raw, err := json.Marshal(q.Instance)
	require.NoError(t, err)
	require.NoError(t, x.f.db.Exec("INSERT INTO proxy_instances(id,environment_id,data) VALUES (?,?,?)", q.Instance.ID, q.Instance.EnvironmentID, raw).Error)
	m := &p.Module{Hub: &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}, Modules: x.manager, Settings: x.hubSettings, Transport: tr}
	m.RegisterRoutes(x.hubAPI, false)
	entered := make(chan struct{})
	release := make(chan struct{})
	x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if strings.HasSuffix(r.URL.Path, "/inventory") {
			close(entered)
			<-release
		}
		next.ServeHTTP(w, r)
	}
	input, err := json.Marshal(dto.OperationRequest{Target: q.Target, Expected: q.Expected, GraphHashes: q.GraphHashes, Change: q.Change})
	require.NoError(t, err)
	path := "/api/modules/proxy/instances/" + q.Instance.ID + "/operations"
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- x.request("POST", path, string(input)) }()
	<-entered
	require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
	close(release)
	got := <-done
	want := m27UnknownResponse("POST", path)
	require.Equal(t, want.Code, got.Code)
	require.Equal(t, want.Body.String(), got.Body.String())
	var n int64
	require.NoError(t, x.f.db.Table("proxy_operations").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, a.DB.Table("proxy_journal").Count(&n).Error)
	require.Zero(t, n)
}

// The engine's diagnostic survives the real agent, the Direct proxy or Edge tunnel and
// the hub record.
func TestM27ProxyDiagnosticRealDirectEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x, a, tr, q := proxyWireFixture(t, mode)
			refusal := dto.Fail(dto.InvalidGraph, 422)
			refusal.Path, refusal.Line, refusal.Detail = "routes/app.caddy", 3, "Error: adapting config: unrecognized directive: example_directive"
			a.Runtime.(*proxyWireRuntime).validateErr = refusal
			h := &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}
			o, err := h.Apply(t.Context(), q)
			require.NoError(t, err)
			want := &dto.Diagnostic{Path: refusal.Path, Line: refusal.Line, Detail: refusal.Detail}
			require.Equal(t, dto.Rejected, o.State)
			require.Equal(t, want, o.Diagnostic)
			stored, err := h.Query(t.Context(), q.ID)
			require.NoError(t, err)
			require.Equal(t, want, stored.Diagnostic)
		})
	}
}
