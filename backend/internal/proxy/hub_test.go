// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original hub reconciliation and migration regression proofs.

package proxy

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

type localTransport struct {
	a                 *Agent
	reg               string
	applyCalls        int
	drop              bool
	noAdmission       bool
	queryErr          error
	releaseErr        error
	fenceGeneration   string
	staleRegistration bool
	beforeApply       func()
	applyErr          error
	queries           atomic.Int32
	block             chan struct{}
	releaseMutate     func(*dto.Operation)
	afterRelease      func()
	applyMutate       func(*dto.Operation)
	fenceErr          error
}

func (t *localTransport) Registration(context.Context, string) (string, error) { return t.reg, nil }
func (t *localTransport) Apply(ctx context.Context, q dto.ApplyRequest) (dto.Operation, error) {
	t.applyCalls++
	if t.beforeApply != nil {
		t.beforeApply()
	}
	if t.applyErr != nil {
		return dto.Operation{}, t.applyErr
	}
	var o dto.Operation
	var err error
	if !t.noAdmission {
		o, err = t.a.Apply(ctx, q)
	}
	if t.staleRegistration {
		t.reg = "example-replaced"
	}
	if t.drop {
		return dto.Operation{}, errors.New("response lost")
	}
	if t.applyMutate != nil {
		t.applyMutate(&o)
	}
	return o, err
}
func (t *localTransport) Query(ctx context.Context, q dto.Query) (dto.Operation, error) {
	t.queries.Add(1)
	if t.block != nil {
		<-t.block
	}
	if t.queryErr != nil {
		return dto.Operation{}, t.queryErr
	}
	return t.a.Query(ctx, q)
}
func (t *localTransport) Fence(ctx context.Context, q dto.Query) (dto.FenceResult, error) {
	if t.fenceErr != nil {
		return dto.FenceResult{}, t.fenceErr
	}
	r, err := t.a.Fence(ctx, q)
	if t.fenceGeneration != "" {
		r.Generation = t.fenceGeneration
	}
	return r, err
}
func (t *localTransport) Release(ctx context.Context, q dto.ReleaseRequest) (dto.Operation, error) {
	if t.releaseErr != nil {
		return dto.Operation{}, t.releaseErr
	}
	o, err := t.a.Release(ctx, q)
	if t.releaseMutate != nil {
		t.releaseMutate(&o)
	}
	if t.afterRelease != nil {
		t.afterRelease()
	}
	return o, err
}
func TestP11OneDispatchGenerationAndRegistration(t *testing.T) {
	for _, mode := range []string{"lost-after-effect", "lost-before-admission", "different-generation", "lost-journal", "different-registration", "wrong-fence-generation"} {
		t.Run(mode, func(t *testing.T) {
			x := newFixture(t)
			tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true}
			switch mode {
			case "lost-before-admission":
				tr.noAdmission = true
			case "different-generation":
				tr.queryErr = dto.Fail(dto.GenerationMismatch, 409)
			case "lost-journal":
				tr.queryErr = dto.Fail(dto.JournalLost, 409)
			case "different-registration":
				tr.staleRegistration = true
			case "wrong-fence-generation":
				tr.noAdmission = true
				tr.fenceGeneration = "other-generation"
			}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			o, err := h.Apply(t.Context(), x.request)
			require.NoError(t, err)
			want := dto.RecoveryRequired
			if mode == "lost-after-effect" {
				want = dto.Verified
			}
			if mode == "lost-before-admission" {
				want = dto.NotApplied
			}
			require.Equal(t, want, o.State)
			require.Equal(t, 1, tr.applyCalls)
			again, err := h.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, want, again.State)
			require.Equal(t, 1, tr.applyCalls, "a persisted ID never dispatches Apply again")
			var n int64
			require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
			if want.Final() {
				require.Zero(t, n)
			} else {
				require.EqualValues(t, 1, n)
			}
		})
	}
}
func TestP24HubReleaseBoundary(t *testing.T) {
	for _, mode := range []string{"same-generation", "generation-mismatch", "unreachable"} {
		t.Run(mode, func(t *testing.T) {
			x := newFixture(t)
			x.a.afterCommit = func(p dto.Phase) error {
				if p == dto.Observing {
					return errCrash
				}
				return nil
			}
			tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true, queryErr: dto.Fail(dto.JournalLost, 409)}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			o, err := h.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, o.State)
			x.a.afterCommit = nil
			tr.queryErr = nil
			x.f.files["route.yml"] = Snapshot([]byte("operator"), true)
			o, err = x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, o.State)
			switch mode {
			case "generation-mismatch":
				tr.releaseErr = dto.Fail(dto.GenerationMismatch, 409)
			case "unreachable":
				tr.releaseErr = errors.New("agent unreachable")
			}
			before := x.f.files["route.yml"]
			o, err = h.Release(t.Context(), x.request.ID, before.FileState, "example-admin")
			var n int64
			require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
			if mode == "unreachable" {
				require.Error(t, err)
				require.EqualValues(t, 1, n)
				require.Equal(t, dto.RecoveryRequired, o.State)
			} else {
				require.NoError(t, err)
				require.Zero(t, n)
				require.Equal(t, dto.Released, o.State)
			}
			var audits int64
			require.NoError(t, x.db.Table("events").Where("message_code = ?", "proxy.operation.released").Count(&audits).Error)
			if mode == "unreachable" {
				require.Zero(t, audits)
			} else {
				require.EqualValues(t, 1, audits)
			}
			require.Equal(t, before, x.f.files["route.yml"])
			var j Journal
			require.NoError(t, x.db.First(&j, "id = ?", x.request.ID).Error)
			if mode == "same-generation" {
				require.Equal(t, dto.Released, j.State)
			} else {
				require.Equal(t, dto.RecoveryRequired, j.State)
			}
		})
	}
}
func TestP20RetentionAndMissingReceipt(t *testing.T) {
	x := newFixture(t)
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	past := time.Now().Add(-31 * 24 * time.Hour)
	require.NoError(t, x.db.Model(&Journal{}).Where("id = ?", o.ID).Update("finished_at", past).Error)
	require.NoError(t, prune(t.Context(), x.db.DB, time.Now()))
	_, err = x.a.Query(t.Context(), x.query())
	require.ErrorContains(t, err, "unknown-operation")
	_, err = x.a.Fence(t.Context(), x.query())
	require.ErrorContains(t, err, "journal-lost", "expired receipts cannot become not-applied")
	second := x.request
	second.ID = "unfinished"
	second.Expected = x.f.files["route.yml"].FileState
	second.GraphHashes, _ = x.f.Graph(t.Context(), second.Instance)
	x.a.afterCommit = func(dto.Phase) error { return errCrash }
	_, err = x.a.Apply(t.Context(), second)
	require.ErrorIs(t, err, errCrash)
	require.NoError(t, x.db.Model(&Journal{}).Where("id = ?", second.ID).Update("created_at", past).Error)
	require.NoError(t, prune(t.Context(), x.db.DB, time.Now()))
	_, err = findJournal(t.Context(), x.db.DB, second.ID)
	require.NoError(t, err)
}
func TestP20UnknownJournalStateFailsSafe(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(dto.Phase) error { return errCrash }
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	require.NoError(t, x.db.Model(&Journal{}).Where("id = ?", x.request.ID).Update("state", "unknown-state").Error)
	_, err = x.a.Query(t.Context(), x.query())
	require.ErrorContains(t, err, "journal-lost")
	id, err := unfinished(t.Context(), x.db.DB, "proxy_journal")
	require.NoError(t, err)
	require.Equal(t, x.request.ID, id)
	require.Zero(t, x.f.publications)
}

func TestHubFinalReceiptDoesNotRegress(t *testing.T) {
	x := newFixture(t)
	tr := &localTransport{a: x.a, reg: x.request.Registration}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	final, err := h.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, final.State)
	late := final
	late.State = dto.Pending
	late.FinishedAt = nil
	got, err := h.accept(t.Context(), x.request, late, late)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, got.State)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestP24ReleaseAfterCredentialRotation(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true, queryErr: dto.Fail(dto.JournalLost, 409)}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	_, err := h.Apply(t.Context(), x.request)
	require.NoError(t, err)
	x.a.afterCommit = nil
	tr.queryErr = nil
	x.f.files[x.request.Target] = Snapshot([]byte("operator"), true)
	_, err = x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	tr.reg = "example-rotated-registration"
	got, err := h.Release(t.Context(), x.request.ID, x.f.files[x.request.Target].FileState, "example-admin")
	require.NoError(t, err)
	require.Equal(t, dto.Released, got.State)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)
	require.Equal(t, "operator", string(x.f.files[x.request.Target].Content))
}
