// Source: docklet-next/issues/390
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original restored-mode retry, held-engine barrier and durable settlement proofs.

package backup

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	engine "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/italypaleale/francis/actor"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Interpose only the engine's state writes; every read/write still reaches the
// real actor host and its store. A held write marks the exact settlement boundary.
type settlementHost struct {
	actor.Host
	service *actor.Service
	write   func(context.Context, string) error
}

func (h *settlementHost) ListStates(ctx context.Context, kind string, opts *actor.ListStatesOpts) (actor.StateList, error) {
	return h.service.ListStates(ctx, kind, opts)
}
func (h *settlementHost) SetState(ctx context.Context, kind, id string, state any, opts *actor.SetStateOpts) error {
	if h.write != nil {
		if err := h.write(ctx, id); err != nil {
			return err
		}
	}
	return h.service.SetState(ctx, kind, id, state, opts)
}
func attachSettlementEngine(t *testing.T, s *Service) *actor.Service {
	t.Helper()
	runtime := francistest.New(t)
	s.deps.Engine = engine.NewEngine(t.Context(), nil, nil)
	s.deps.Engine.SetExecutionReady(func() bool { return false })
	require.NoError(t, s.deps.Engine.Register(runtime))
	francistest.Start(t, runtime)
	return runtime.Service()
}
func seedSettlement(t *testing.T, service *actor.Service, id, name string) {
	t.Helper()
	state := dto.DurableRunState{Command: dto.DurableRunCommand{RunID: id, Kind: "volume", Payload: []byte(`{"volumeName":"` + name + `"}`)}, Status: scheduler.Running, Started: true}
	require.NoError(t, service.SetState(t.Context(), "backup-run", id, state, nil))
}
func settlementState(t *testing.T, service *actor.Service, id string) dto.DurableRunState {
	t.Helper()
	var state dto.DurableRunState
	require.NoError(t, service.GetState(t.Context(), "backup-run", id, &state))
	return state
}
func restoredService(t *testing.T, restarted *[]string, effects *atomic.Int32) *Service {
	t.Helper()
	return startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		case strings.HasSuffix(r.URL.Path, "/containers/archived/json"):
			effects.Add(1)
			_, _ = w.Write([]byte(`{"Id":"archived","State":{"Running":false}}`))
		case strings.HasSuffix(r.URL.Path, "/volumes/foreign"):
			effects.Add(1)
			_, _ = w.Write([]byte(`{"Name":"foreign","Labels":{"io.arcane.restore":"other"}}`))
		default:
			t.Errorf("unexpected Docker effect %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}), restarted)
}
func requireAdmissionClosed(t *testing.T, s *Service) {
	t.Helper()
	require.ErrorContains(t, s.checkWriterFence(t.Context(), "unrelated"), "volume.backup.writer-unconfirmed", "admissions must remain closed through complete settlement")
}

func TestRestoredSettlementHoldsAdmissionThroughEngineAndSettlesEligibleIntents(t *testing.T) {
	restarted := []string{}
	var effects atomic.Int32
	s := restoredService(t, &restarted, &effects)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	service := attachSettlementEngine(t, s)
	seedSettlement(t, service, "old", "data")
	seedSettlement(t, service, "blocked", "unconfirmed")
	// Introduce an unconfirmed writer after discovery, at the cleaner boundary.
	s.SetInputCleaner(func(context.Context, []string) error {
		s.fenceMu.Lock()
		s.writerFences["unconfirmed"] = true
		delete(s.confirmedWriters, "unconfirmed")
		s.fenceMu.Unlock()
		return nil
	})
	original := *service
	entered, release := make(chan struct{}), make(chan struct{})
	held := &settlementHost{service: &original, write: func(_ context.Context, id string) error {
		if id == "old" {
			close(entered)
			<-release
		}
		return nil
	}}
	*service = *actor.NewService(held)
	defer func() { *service = original }()
	done := make(chan error, 1)
	go func() { done <- s.ReconcileStartup(t.Context(), true) }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("engine must settle a nonempty eligible intent while global admission is closed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not reach settlement write")
	}
	// Read the actual durable scheduler state before allowing the write to commit.
	require.Equal(t, scheduler.Running, settlementState(t, &original, "old").Status)
	admissionErr := s.checkWriterFence(t.Context(), "unrelated")
	s.fenceMu.Lock()
	pending := s.restoredPending
	settled := s.settledInitial
	s.fenceMu.Unlock()
	// Release before fatal assertions so every mutation remains joinable.
	close(release)
	require.NoError(t, <-done)
	require.ErrorContains(t, admissionErr, "volume.backup.writer-unconfirmed", "admissions must remain closed through complete settlement")
	require.True(t, pending)
	require.False(t, settled)
	require.Equal(t, scheduler.Failed, settlementState(t, &original, "old").Status, "eligible old intents must actually settle")
	require.Equal(t, "volume.backup.interrupted", settlementState(t, &original, "old").Error)
	require.Equal(t, scheduler.Running, settlementState(t, &original, "blocked").Status, "per-volume writer fences still govern engine settlement")
	require.NoError(t, s.checkWriterFence(t.Context(), "unrelated"))
	require.Empty(t, restarted)
	require.Zero(t, effects.Load())
}

func TestRestoredSettlementFailureRetainsModeAcrossFalseRetries(t *testing.T) {
	for _, failure := range []string{"sql", "lifecycle", "cleaner", "engine"} {
		for _, retry := range []string{"admission", "timer"} {
			t.Run(failure+"/"+retry, func(t *testing.T) {
				restarted := []string{}
				var effects atomic.Int32
				s := restoredService(t, &restarted, &effects)
				require.NoError(t, s.ReconcileStartup(t.Context(), false))
				service := attachSettlementEngine(t, s)
				seedSettlement(t, service, "old", "data")
				row := &volume.Backup{ID: "archived", VolumeName: "data", Status: "running", CreatedAt: time.Now().UTC(), StoppedContainers: []volume.StoppedContainer{{ID: "archived"}}}
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(row).Error)
				for _, run := range []restore.Run{
					{ID: "preparing", VolumeName: "data", Status: "running", Phase: "preparing", Mode: "replace", StoppedContainers: new(`[{"ID":"archived"}]`)},
					{ID: "new", VolumeName: "foreign", Status: "running", Phase: "writing", Mode: "new", CreatedVolume: true, CleanupPending: true, NewVolumeName: "foreign"},
					{ID: "writing", VolumeName: "data", Status: "running", Phase: "writing", Mode: "replace", PendingRestart: true},
				} {
					require.NoError(t, s.deps.DB.Create(&run).Error)
				}
				injected := errors.New("synthetic settlement failure")
				fail := true
				save := s.deps.Store.Save
				s.deps.Store.Save = func(ctx context.Context, row *volume.Backup) error {
					if failure == "sql" && fail {
						return injected
					}
					return save(ctx, row)
				}
				require.NoError(t, s.deps.DB.Callback().Update().Before("gorm:update").Register("test:restored-failure", func(tx *gorm.DB) {
					if failure == "lifecycle" && fail && tx.Statement.Table == "volume_restore_runs" {
						tx.AddError(injected)
					}
				}))
				s.SetInputCleaner(func(context.Context, []string) error {
					if failure == "cleaner" && fail {
						return injected
					}
					return nil
				})
				original := *service
				*service = *actor.NewService(&settlementHost{service: &original, write: func(context.Context, string) error {
					if failure == "engine" && fail {
						return injected
					}
					return nil
				}})
				defer func() { *service = original }()
				require.ErrorIs(t, s.ReconcileStartup(t.Context(), true), injected)
				require.True(t, s.WriterFenced("unrelated"), "a failed restored pass keeps the global barrier closed")
				requireAdmissionClosed(t, s) // Re-enters the actual false admission retry while failure persists.
				require.Empty(t, restarted, "archived consumers must never restart on a false retry")
				require.True(t, s.restoredPending, "false retries must retain restored classification")
				require.Zero(t, effects.Load(), "archived history must not inspect consumers or attempt foreign-volume cleanup")
				fail = false
				if retry == "admission" {
					require.NoError(t, s.checkWriterFence(t.Context(), "unrelated"))
				} else {
					// The minute loop's exact entrypoint, without waiting on wall-clock time.
					require.NoError(t, s.ReconcileStartup(t.Context(), false))
				}
				require.False(t, s.WriterFenced("unrelated"))
				require.False(t, s.restoredPending)
				require.Empty(t, restarted)
				require.Zero(t, effects.Load())
				var backup volume.Backup
				require.NoError(t, s.deps.DB.Table("volume_backups").First(&backup, "id = ?", "archived").Error)
				require.Equal(t, "failed", backup.Status)
				var runs []restore.Run
				require.NoError(t, s.deps.DB.Find(&runs).Error)
				require.Len(t, runs, 3)
				for _, run := range runs {
					require.Equal(t, "failed", run.Status)
					require.False(t, run.PendingRestart)
					require.False(t, run.CleanupPending)
				}
				require.Equal(t, scheduler.Failed, settlementState(t, &original, "old").Status)
			})
		}
	}
}

func TestRestoredSettlementResetsCompletedStartupBarrier(t *testing.T) {
	restarted := []string{}
	var effects atomic.Int32
	s := restoredService(t, &restarted, &effects)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.False(t, s.WriterFenced("unrelated"))
	entered, release := make(chan struct{}), make(chan struct{})
	s.SetInputCleaner(func(context.Context, []string) error { close(entered); <-release; return nil })
	done := make(chan error, 1)
	go func() { done <- s.ReconcileStartup(t.Context(), true) }()
	<-entered
	err := s.checkWriterFence(t.Context(), "unrelated")
	close(release)
	require.NoError(t, <-done)
	require.ErrorContains(t, err, "volume.backup.writer-unconfirmed", "restored discovery must reset the completed initial settlement barrier")
	require.False(t, s.WriterFenced("unrelated"))
}

// A routine retry settles only the volumes it recovered; a live run of another volume stays running.
func TestRoutineRetryLeavesLiveRunOfUnrecoveredVolume(t *testing.T) {
	restarted := []string{}
	var effects atomic.Int32
	s := restoredService(t, &restarted, &effects)
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	service := attachSettlementEngine(t, s)
	seedSettlement(t, service, "live", "data")
	require.NoError(t, s.ReconcileStartup(t.Context(), false))
	require.Equal(t, scheduler.Running, settlementState(t, service, "live").Status, "routine retry must not settle a live run")
}
