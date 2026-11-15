// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original S2 R1 lifecycle proofs: caller cancellation, identity change, adoption, settlement and intent roundtrip.

package proxy

import (
	"context"
	"encoding/json/jsontext"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

func persistRecord(t *testing.T, x *fixture, state dto.State) {
	t.Helper()
	d, err := Digest(x.request)
	require.NoError(t, err)
	o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.request.Generation, Registration: x.request.Registration, Digest: d, State: state, Phase: dto.Admitted, Target: x.request.Target, Old: x.request.Expected, CreatedAt: time.Now().UTC()}
	request, err := marshal(x.request)
	require.NoError(t, err)
	result, err := marshal(o)
	require.NoError(t, err)
	require.NoError(t, x.db.Create(&Record{ID: o.ID, InstanceID: o.InstanceID, Generation: o.Generation, Digest: d, Registration: o.Registration, State: state, Request: request, Result: result, CreatedAt: o.CreatedAt}).Error)
	require.NoError(t, x.db.Create(&reservation{InstanceID: o.InstanceID, OperationID: o.ID}).Error)
}

// L1: a caller that disconnects after admission does not change the outcome.
func TestL1AdmittedWorkOutlivesTheCaller(t *testing.T) {
	x := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Admitted {
			cancel()
		}
		return nil
	}
	o, err := x.a.Apply(ctx, x.request)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State, "reason %s", o.Reason)
	require.Equal(t, 1, x.f.publications)
	require.Equal(t, x.e.new, x.f.files["route.yml"])
}

// L2: the hub dispatch outlives the caller and settles the record.
func TestL2HubDispatchOutlivesTheCaller(t *testing.T) {
	x := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	tr := &localTransport{a: x.a, reg: x.request.Registration, beforeApply: cancel}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	_, _ = h.Apply(ctx, x.request)
	var row Record
	require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
	require.Equal(t, dto.Verified, row.State)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)
}

// L2: reconciliation runs asynchronously, one pass per environment, and settles pending records.
func TestL2ReconcilerIsAsynchronousAndSingle(t *testing.T) {
	x := newFixture(t)
	tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true, queryErr: dto.Fail(dto.AgentUnavailable, 503)}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	o, err := h.Apply(t.Context(), x.request)
	require.Error(t, err)
	require.Equal(t, dto.Pending, o.State)
	tr.queryErr = nil
	tr.queries.Store(0)
	tr.block = make(chan struct{})
	release := sync.OnceFunc(func() { close(tr.block) })
	defer release()
	m := &Module{Hub: h}
	returned := make(chan struct{})
	go func() {
		m.reconcileAsync(x.request.Instance.EnvironmentID)
		m.reconcileAsync(x.request.Instance.EnvironmentID)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		release()
		require.Fail(t, "reconciliation must not block its caller")
	}
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, 1, tr.queries.Load(), "a second pass for the same environment must not start")
	release()
	require.Eventually(t, func() bool {
		var row Record
		return x.db.First(&row, "id = ?", x.request.ID).Error == nil && row.State == dto.Verified
	}, 5*time.Second, 10*time.Millisecond)
}

// L3: final receipts are reported after the container identity changed.
func TestL3FinalReceiptSurvivesIdentityChange(t *testing.T) {
	x := newFixture(t)
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	x.f.checkErr = dto.Fail(dto.ContainerChanged, 409)
	got, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.Verified, got.State)
	fenced, err := x.a.Fence(t.Context(), x.query())
	require.NoError(t, err)
	require.NotNil(t, fenced.Operation)
	require.Equal(t, dto.Verified, fenced.Operation.State)
}

func recoveryRequired(t *testing.T, x *fixture) {
	t.Helper()
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	x.f.files["route.yml"] = Snapshot([]byte("operator"), true)
	o, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
}

// L4: Release rebinds a recreated container with an unchanged source, and refuses a changed source.
func TestL4ReleaseRebindsUnchangedSource(t *testing.T) {
	for _, changed := range []bool{false, true} {
		x := newFixture(t)
		recoveryRequired(t, x)
		x.f.checkErr = dto.Fail(dto.ContainerChanged, 409)
		if changed {
			x.f.rebindErr = dto.Fail(dto.SourceReadOnly, 409)
		}
		confirmed := x.f.files["route.yml"].FileState
		o, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Confirmed: confirmed, Actor: "example-admin"})
		if changed {
			require.Error(t, err)
			require.Equal(t, dto.RecoveryRequired, o.State)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, dto.Released, o.State)
		require.True(t, x.f.rebound)
	}
}

// L5: the hub adopts the agent's final receipt over its own recovery-required, and
// treats journal-lost like a generation mismatch.
func TestL5HubAdoptsFinalReceiptOnRelease(t *testing.T) {
	x := newFixture(t)
	tr := &localTransport{a: x.a, reg: x.request.Registration, staleRegistration: true}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	o, err := h.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State, "registration changed during the dispatch")
	tr.reg = x.request.Registration
	o, err = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)

	y := newFixture(t)
	recoveryRequired(t, y)
	tr = &localTransport{a: y.a, reg: y.request.Registration, releaseErr: dto.Fail(dto.JournalLost, 409)}
	h = &Hub{DB: y.db.DB, Modules: y.mods, Transport: tr}
	persistRecord(t, y, dto.RecoveryRequired)
	o, err = h.Release(t.Context(), y.request.ID, y.f.files["route.yml"].FileState, "example-admin")
	require.NoError(t, err)
	require.Equal(t, dto.Released, o.State)
}

// L6: undecodable journal content neither fails startup nor keeps the source locked after Release.
func TestL6SettlementSurvivesUndecodableRows(t *testing.T) {
	x := newFixture(t)
	require.NoError(t, x.db.Create(&Journal{ID: "broken-one", Source: "host\x00" + x.request.Instance.Source.Canonical, Generation: x.a.Generation(), Digest: "example", Registration: x.request.Registration, State: dto.Pending, Phase: dto.Staging, Intent: []byte("{"), Result: []byte("{"), CreatedAt: time.Now().UTC()}).Error)
	require.NoError(t, x.a.Settle(t.Context()))
	q := dto.Query{ID: "broken-one", Instance: x.request.Instance, Generation: x.a.Generation(), Registration: x.request.Registration}
	_, err := x.a.Query(t.Context(), q)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.JournalLost, p.Code)
	o, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: q, Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
	require.NoError(t, err)
	require.Equal(t, dto.Released, o.State)
	got, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, got.State, "the source lock must be free after the release")
}

// I1: engine state and the publication time survive a restart.
func TestI1IntentRoundtripsEngineStateAndPublicationTime(t *testing.T) {
	x := newFixture(t)
	x.e.state = jsontext.Value(`{"probe":"before"}`)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	require.NoError(t, x.a.Settle(t.Context()))
	require.NotEmpty(t, x.e.seen)
	last := x.e.seen[len(x.e.seen)-1]
	require.JSONEq(t, `{"probe":"before"}`, string(last.Engine))
	require.False(t, last.PublishedAt.IsZero())
}

// S1 N5: an agent follows the hub's switch-off even with its own unfinished journal.
func TestAgentFollowsHubSwitchOff(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	require.NoError(t, x.mods.Register(features.Proxy, modules.Hooks{Unfinished: func(ctx context.Context) (string, error) { return unfinished(ctx, x.db.DB, "proxy_journal") }}))
	x.mods.SetAgentMode(true)
	_, err = x.settings.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
	require.NoError(t, err)
	require.False(t, x.mods.Enabled(t.Context(), features.Proxy))
}

// N2 and N8: a typed refusal is kept as the not-applied reason; a replayed ID after pruning is a typed conflict.
func TestRefusalReasonAndReplayAfterPrune(t *testing.T) {
	x := newFixture(t)
	tr := &localTransport{a: x.a, reg: x.request.Registration, applyErr: dto.Fail(dto.SourceReadOnly, 409)}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	o, err := h.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.NotApplied, o.State)
	require.Equal(t, dto.SourceReadOnly, o.Reason)

	y := newFixture(t)
	_, err = y.a.Apply(t.Context(), y.request)
	require.NoError(t, err)
	require.NoError(t, y.db.Exec("DELETE FROM proxy_journal").Error)
	_, err = y.a.Apply(t.Context(), y.request)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, dto.OperationConflict, p.Code)
}
