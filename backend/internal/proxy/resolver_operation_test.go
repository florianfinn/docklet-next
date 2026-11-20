// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bytes"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"log/slog"
)

type resolverHost struct {
	target            resolver.Target
	identity          dto.Instance
	recreates         int
	fail, restoreFail bool
	prepareErr        error
}

func (f *resolverHost) Prepare(_ context.Context, q dto.ResolverApplyRequest) (resolver.Target, error) {
	if f.prepareErr != nil {
		return f.target, f.prepareErr
	}
	for j := range f.target.Files {
		file := &f.target.Files[j]
		file.After = append([]byte(nil), file.Before...)
		if file.Kind == "compose" {
			file.After = append(file.After, []byte("# resolver\n")...)
		}
		if file.Kind == "env" {
			for _, v := range q.Request.Variables {
				if v.Value != nil {
					file.After = append(file.After, []byte(v.Name+"='"+*v.Value+"'\n")...)
				}
			}
		}
		file.New = Snapshot(file.After, true).FileState
	}
	return f.target, nil
}
func (f *resolverHost) Preflight(context.Context, resolver.Target) error { return nil }
func (f *resolverHost) Recreate(context.Context, resolver.Target) error {
	f.recreates++
	if f.recreates == 2 && f.restoreFail {
		return errors.New("submitted-sentinel")
	}
	f.identity.Identity.ContainerID = "replacement-" + string(rune('a'+f.recreates))
	f.identity.Identity.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return nil
}
func (f *resolverHost) Settle(context.Context, resolver.Target) error { return nil }
func (f *resolverHost) Replacement(context.Context, resolver.Target, dto.Instance) (dto.Instance, error) {
	return f.identity, nil
}
func (f *resolverHost) Observe(_ context.Context, _ dto.Instance, _ string, present bool, _ time.Time) error {
	if present && f.fail {
		return dto.Fail(dto.ProxyRejected, 422)
	}
	return nil
}
func resolverFixture(t *testing.T) (*fixture, *resolverHost, dto.ResolverApplyRequest) {
	x := newFixture(t)
	q := dto.ResolverApplyRequest{ID: "resolver-one", Instance: x.request.Instance, Generation: x.a.Generation(), Registration: x.request.Registration, Request: dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "Example", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}}, Variables: []dto.ResolverVariable{}}}
	q.Instance.Authority = dto.FileAuthority
	value := "submitted-sentinel"
	q.Request.Variables = []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &value}}
	dir := t.TempDir()
	q.Instance.Source.Canonical = dir
	q.Instance.Source.ProjectPath = dir
	f := &resolverHost{target: resolver.Target{ProjectID: "project-one", Directory: dir, Compose: "compose.yaml", Project: "example", Service: "gateway"}, identity: q.Instance}
	for _, p := range []string{"compose.yaml", ".env"} {
		content := "INLINE=inline-sentinel\n"
		kind := "compose"
		mode := os.FileMode(0644)
		if p == ".env" {
			content = "REUSED='reused-sentinel'\n"
			kind = "env"
			mode = 0600
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, p), []byte(content), mode))
		file, err := resolver.Read(dir, p)
		require.NoError(t, err)
		file.Kind = kind
		f.target.Files = append(f.target.Files, file)
	}
	x.a.ResolverHost = f
	x.a.ResolverBackups = filepath.Join(t.TempDir(), "backups")
	m := &Module{Hub: &Hub{DB: x.db.DB}}
	require.NoError(t, m.saveInstance(t.Context(), q.Instance))
	return x, f, q
}
func TestR15ResolverReplayBindingAndSecrets(t *testing.T) {
	x, f, q := resolverFixture(t)
	q.Instance.Source.ProjectPath = "/example/untrusted-root"
	first, err := x.a.CreateResolver(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, dto.Applied, first.State)
	stored, err := findJournal(t.Context(), x.db.DB, q.ID)
	require.NoError(t, err)
	var persisted dto.ResolverEnvelope
	require.NoError(t, json.Unmarshal(stored.Intent, &persisted))
	require.Equal(t, q.Instance.Source.Canonical, persisted.Instance.Source.ProjectPath)
	changed := "different-live-sentinel"
	q.Request.Variables[0].Value = &changed
	again, err := x.a.CreateResolver(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, 1, f.recreates)
	q.Instance.ID = "other-instance"
	_, err = x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.OperationConflict)
	q.Instance.ID = first.InstanceID
	q.Request.Name = "Other"
	_, err = x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.OperationConflict)
	for _, table := range []string{"proxy_journal", "proxy_admissions"} {
		var rows []map[string]any
		require.NoError(t, x.db.Table(table).Find(&rows).Error)
		raw, _ := json.Marshal(rows)
		for _, sentinel := range []string{"submitted-sentinel", "different-live-sentinel", "inline-sentinel", "reused-sentinel"} {
			require.NotContains(t, string(raw), sentinel)
		}
	}
	var journals []Journal
	require.NoError(t, x.db.Find(&journals).Error)
	for _, j := range journals {
		for _, b := range [][]byte{j.Intent, j.Result} {
			require.NotContains(t, string(b), "submitted-sentinel")
			require.NotContains(t, string(b), "inline-sentinel")
			require.NotContains(t, string(b), "reused-sentinel")
		}
	}
	require.Equal(t, "[redacted]", resolver.Redact(changed, []dto.ResolverVariable{{Value: &changed}}))
}

type resolverTransport struct {
	*localTransport
	resolverCalls int
	before        func()
	resolverDrop  bool
}

func (t *resolverTransport) CreateResolver(ctx context.Context, q dto.ResolverApplyRequest) (dto.Operation, error) {
	t.resolverCalls++
	if t.before != nil {
		t.before()
	}
	o, e := t.a.CreateResolver(ctx, q)
	if t.resolverDrop {
		return dto.Operation{}, errors.New("lost response")
	}
	return o, e
}
func TestR5R16HubAdoptsDispatchQueryAndReconciler(t *testing.T) {
	for _, mode := range []string{"dispatch", "query", "reconciler", "rollback", "cas-conflict"} {
		t.Run(mode, func(t *testing.T) {
			x, f, q := resolverFixture(t)
			f.fail = mode == "rollback"
			tr := &resolverTransport{localTransport: &localTransport{a: x.a, reg: q.Registration}, resolverDrop: mode == "query"}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			if mode == "cas-conflict" {
				tr.before = func() {
					var row instanceRow
					require.NoError(t, x.db.First(&row, "id = ?", q.Instance.ID).Error)
					var i dto.Instance
					require.NoError(t, json.Unmarshal(row.Data, &i))
					i.Identity.ContainerID = "concurrent-container"
					b, _ := marshal(i)
					require.NoError(t, x.db.Model(&row).Update("data", b).Error)
				}
			}
			var result dto.Operation
			var err error
			if mode == "reconciler" {
				// Model hub exit after dispatch by writing exactly its redacted pending row.
				en := resolverEnvelope(q)
				d, e := en.Digest()
				require.NoError(t, e)
				o := dto.Operation{Kind: dto.ResolverOperation, ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Phase: dto.Admitted, CreatedAt: time.Now().UTC(), Resolver: &dto.ResolverReceipt{Intent: en.Intent, Phase: dto.ResolverAdmitted, Files: []dto.ResolverFile{}}}
				request, _ := marshal(en)
				raw, _ := marshal(o)
				require.NoError(t, x.db.Create(&Record{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Request: request, Result: raw, CreatedAt: o.CreatedAt}).Error)
				require.NoError(t, x.db.Create(&reservation{InstanceID: q.Instance.ID, OperationID: q.ID}).Error)
				_, err = x.a.CreateResolver(t.Context(), q)
				require.NoError(t, err)
				m := &Module{Hub: h}
				m.reconcile(t.Context(), q.Instance.EnvironmentID)
				result, err = h.Query(t.Context(), q.ID)
			} else {
				result, err = h.CreateResolver(t.Context(), q)
			}
			require.NoError(t, err)
			var row instanceRow
			require.NoError(t, x.db.First(&row, "id = ?", q.Instance.ID).Error)
			var adopted dto.Instance
			require.NoError(t, json.Unmarshal(row.Data, &adopted))
			if mode == "cas-conflict" {
				// B1: no adoption, but the final receipt persists and frees the reservation.
				require.Equal(t, dto.Applied, result.State)
				require.Equal(t, "concurrent-container", adopted.Identity.ContainerID)
				var held int64
				require.NoError(t, x.db.Model(&reservation{}).Where("operation_id = ?", q.ID).Count(&held).Error)
				require.Zero(t, held)
				return
			}
			require.Contains(t, []dto.State{dto.Applied, dto.RolledBack}, result.State)
			require.Equal(t, f.identity.Identity, adopted.Identity)
			before := f.recreates
			_, err = h.CreateResolver(t.Context(), q)
			require.NoError(t, err)
			require.Equal(t, before, f.recreates)
			if mode == "reconciler" {
				require.Zero(t, tr.resolverCalls)
			} else {
				require.Equal(t, 1, tr.resolverCalls)
			}
			var records []Record
			require.NoError(t, x.db.Find(&records).Error)
			for _, record := range records {
				require.NotContains(t, string(record.Request), "submitted-sentinel")
				require.NotContains(t, string(record.Result), "submitted-sentinel")
			}
		})
	}
}
func TestR15FenceAndR18UnknownRowNoiseRelease(t *testing.T) {
	x, _, q := resolverFixture(t)
	_, err := x.a.Fence(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, err)
	_, err = x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.Fenced)
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(old)
	receipt := dto.Operation{ID: "unknown-one", InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, State: dto.Pending, Phase: dto.Admitted, CreatedAt: time.Now().UTC()}
	raw, _ := marshal(receipt)
	require.NoError(t, x.db.Create(&Journal{ID: receipt.ID, Source: "unknown", Generation: q.Generation, Registration: q.Registration, State: dto.Pending, Phase: dto.Admitted, Intent: []byte(`{"kind":"future"}`), Result: raw, CreatedAt: receipt.CreatedAt}).Error)
	require.NoError(t, x.a.Settle(t.Context()))
	require.NotContains(t, logs.String(), "registration-changed")
	require.NoError(t, x.db.Create(&Record{ID: receipt.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, State: dto.Pending, Request: []byte(`broken`), Result: raw, CreatedAt: receipt.CreatedAt}).Error)
	h := &Hub{DB: x.db.DB, Transport: &localTransport{a: x.a, reg: q.Registration}}
	_, err = h.Release(t.Context(), receipt.ID, dto.FileState{}, "admin-one")
	require.NoError(t, err)
	var events []event.Event
	require.NoError(t, x.db.Where("resource_id = ?", q.Instance.ID).Find(&events).Error)
	require.Len(t, events, 1)
	require.NotNil(t, events[0].EnvironmentID)
	require.Equal(t, q.Instance.EnvironmentID, *events[0].EnvironmentID)
}
func TestN5ResolverFinalReleaseChecksBindingFirst(t *testing.T) {
	x, _, q := resolverFixture(t)
	_, err := x.a.CreateResolver(t.Context(), q)
	require.NoError(t, err)
	_, err = x.a.Release(t.Context(), dto.ReleaseRequest{Query: dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: "other-registration"}})
	requireCode(t, err, dto.RegistrationChanged)
}

func TestR12RecoveryKeepsReservationAndSourceLock(t *testing.T) {
	x, f, q := resolverFixture(t)
	f.fail = true
	f.restoreFail = true
	tr := &resolverTransport{localTransport: &localTransport{a: x.a, reg: q.Registration}}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	held, err := h.CreateResolver(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, held.State)
	var count int64
	require.NoError(t, x.db.Model(&reservation{}).Where("operation_id = ?", q.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
	next := q
	next.ID = "resolver-two"
	_, err = x.a.CreateResolver(t.Context(), next)
	requireCode(t, err, dto.ModuleBusy)
	released, err := h.Release(t.Context(), held.ID, *held.Current, "admin-one")
	require.NoError(t, err)
	require.Equal(t, dto.Released, released.State)
	require.NoError(t, x.db.Model(&reservation{}).Where("operation_id = ?", q.ID).Count(&count).Error)
	require.Zero(t, count)
	_, err = x.a.CreateResolver(t.Context(), next)
	require.NoError(t, err)
}
func TestR13InvalidResolverHasNoEffects(t *testing.T) {
	x, f, q := resolverFixture(t)
	q.Request.Name = "invalid/name"
	_, err := x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.InvalidInput)
	require.Zero(t, f.recreates)
	var count int64
	require.NoError(t, x.db.Model(&Journal{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestR15HubRestartBeforeDispatchFencesWithoutResend(t *testing.T) {
	x, f, q := resolverFixture(t)
	en := resolverEnvelope(q)
	d, err := en.Digest()
	require.NoError(t, err)
	o := dto.Operation{ID: q.ID, InstanceID: q.Instance.ID, Kind: dto.ResolverOperation, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Phase: dto.Admitted, Resolver: &dto.ResolverReceipt{Intent: en.Intent, Phase: dto.ResolverAdmitted, Files: []dto.ResolverFile{}}}
	raw, _ := marshal(o)
	request, _ := marshal(en)
	require.NoError(t, x.db.Create(&Record{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: d, State: dto.Pending, Request: request, Result: raw}).Error)
	tr := &resolverTransport{localTransport: &localTransport{a: x.a, reg: q.Registration}}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	result, err := h.Query(t.Context(), q.ID)
	require.NoError(t, err)
	require.Equal(t, dto.NotApplied, result.State)
	require.Zero(t, tr.resolverCalls)
	require.Zero(t, f.recreates)
	_, err = x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.Fenced)
}
func TestR7AgentDatabaseAndWALNeverStoreResolverBytes(t *testing.T) {
	for _, outcome := range []string{"applied", "rolled-back", "recovery-required"} {
		t.Run(outcome, func(t *testing.T) {
			var captured bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&captured, nil)))
			defer slog.SetDefault(previous)
			x, f, q := resolverFixture(t)
			f.fail = outcome != "applied"
			f.restoreFail = outcome == "recovery-required"
			receipt, err := x.a.CreateResolver(t.Context(), q)
			require.NoError(t, err)
			replay, err := x.a.CreateResolver(t.Context(), q)
			require.NoError(t, err)
			for _, value := range []any{receipt, replay} {
				raw, e := marshal(value)
				require.NoError(t, e)
				for _, sentinel := range []string{"submitted-sentinel", "inline-sentinel", "reused-sentinel"} {
					require.NotContains(t, string(raw), sentinel)
				}
			}
			for _, sentinel := range []string{"submitted-sentinel", "inline-sentinel", "reused-sentinel"} {
				require.NotContains(t, captured.String(), sentinel)
			}
			var databases []struct{ Name, File string }
			require.NoError(t, x.db.Raw("PRAGMA database_list").Scan(&databases).Error)
			for _, db := range databases {
				if db.Name != "main" {
					continue
				}
				for _, p := range []string{db.File, db.File + "-wal"} {
					raw, err := os.ReadFile(p)
					if os.IsNotExist(err) {
						continue
					}
					require.NoError(t, err)
					for _, sentinel := range []string{"submitted-sentinel", "inline-sentinel", "reused-sentinel"} {
						require.NotContains(t, string(raw), sentinel)
					}
				}
			}
		})
	}
}

func TestR4AgentRestartSettlesResolverWithoutForwardResume(t *testing.T) {
	x, f, q := resolverFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Phase(dto.ResolverObserving) {
			return errCrash
		}
		return nil
	}
	_, err := x.a.CreateResolver(t.Context(), q)
	require.ErrorIs(t, err, resolver.ErrInterrupted)
	backups := x.a.ResolverBackups
	restarted, err := NewAgent(x.db.DB, x.f, x.a.Engines)
	require.NoError(t, err)
	restarted.ResolverHost = f
	restarted.ResolverBackups = backups
	require.NoError(t, restarted.Settle(t.Context()))
	result, err := restarted.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, err)
	require.Equal(t, dto.RolledBack, result.State)
	require.Equal(t, 2, f.recreates)
	for _, file := range f.target.Files {
		current, e := resolver.Read(f.target.Directory, file.Path)
		require.NoError(t, e)
		require.Equal(t, file.Old, current.Old)
	}
	_, err = os.Stat(filepath.Join(backups, q.ID))
	require.True(t, os.IsNotExist(err))
	next := q
	next.ID = "resolver-after-restart"
	next.Instance.Identity = f.identity.Identity
	outcome, err := restarted.CreateResolver(t.Context(), next)
	require.NoError(t, err)
	require.Equal(t, dto.Applied, outcome.State)
}
