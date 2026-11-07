// Source: docklet-next/issues/367
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original bounded startup recovery proofs with an unavailable or silent Docker daemon.

package container

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

const recoveryWorkerID = "9999999999999999999999999999999999999999999999999999999999999999"

// recoveryFault sits in front of the fixture daemon and breaks container
// inspects: "unavailable" answers 503, "silent" never answers.
type recoveryFault struct {
	mu       sync.Mutex
	mode     string
	remain   int // inspects still broken; negative breaks all
	inspects int
}

func (f *recoveryFault) set(mode string, remain int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode, f.remain = mode, remain
}

func (f *recoveryFault) brokenInspects() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inspects
}

func (f *recoveryFault) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		mode := ""
		if strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") && f.remain != 0 {
			mode = f.mode
			f.inspects++
			if f.remain > 0 {
				f.remain--
			}
		}
		f.mu.Unlock()
		switch mode {
		case "unavailable":
			http.Error(w, "daemon unavailable", http.StatusServiceUnavailable)
		case "silent":
			<-r.Context().Done()
		default:
			next(w, r)
		}
	}
}

type recoveryFixture struct {
	s     *ContainerService
	d     *updateDaemon
	db    *database.DB
	rt    *activity.RuntimeService
	store activity.RuntimeStoreBinding
	fault *recoveryFault
	runs  map[string]string // container name -> run ID
}

func withRecoveryTiming(t *testing.T, budget, call, pause time.Duration) {
	t.Helper()
	saved := recreateRecoveryTiming
	recreateRecoveryTiming.budget, recreateRecoveryTiming.call, recreateRecoveryTiming.pause = budget, call, pause
	t.Cleanup(func() { recreateRecoveryTiming = saved })
}

// newRecoveryFixture seeds one image-update recreate per container that the
// prior process left after renaming the original to its backup name.
func newRecoveryFixture(t *testing.T, names ...string) *recoveryFixture {
	t.Helper()
	s, d, _, db := updateFixture(t)
	fault := &recoveryFault{}
	server := httptest.NewServer(fault.wrap(d.serve))
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	s.dockerService = docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(c)

	adapter := activity.NewRuntimeServiceAdapter(nil, s)
	rt := activity.NewRuntimeService(activity.NewActivityService(db, nil), adapter)
	generation, err := rt.InitializeGeneration(t.Context())
	require.NoError(t, err)
	store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation, EnvironmentID: "0", RegistrationID: "registration", ManagerID: "manager"}
	require.NoError(t, rt.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: adapter}))

	f := &recoveryFixture{s: s, d: d, db: db, rt: rt, store: store, fault: fault, runs: map[string]string{}}
	stamp := time.Now().UTC().Add(-time.Minute)
	for _, name := range names {
		id := updateOldID
		if name != "app" {
			id = recoveryWorkerID
			d.containers[id] = dc.InspectResponse{ID: id, Image: updateOldImage, State: &dc.State{}, Config: &dc.Config{Image: updateRef, Labels: map[string]string{}}}
		}
		run := uuid.NewString()
		f.runs[name] = run
		row := activity.RuntimeReceipt{RunID: run, Kind: "receipt", StoreGeneration: generation, ExecutorGeneration: generation, EnvironmentID: "0", RegistrationID: "registration", ManagerID: "manager", RequestDigest: strings.Repeat("a", 64), CallerID: new("operator"), CallerName: new("Operator"), OperationID: new("update-container"), TargetKind: new("container"), TargetID: new(id), TargetName: new(name), GateScope: new("container:" + id), GateKey: new("update-container"), AdmittedAt: &stamp, DispatchState: new("pending"), ExecutorState: new("executing"), ExecutorStartedAt: &stamp, FinalWriteAt: stamp, OutputNextSeq: 1}
		require.NoError(t, db.Create(&row).Error)
		model := activity.Activity{EnvironmentID: "0", Type: activityDTO.TypeContainerRedeploy, Status: activityDTO.StatusRunning, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedAt: stamp, GateScope: *row.GateScope, GateKey: *row.GateKey}
		model.ID = run
		require.NoError(t, db.Create(&model).Error)
		require.NoError(t, s.replacePreviousImage(t.Context(), name, updateOldImage, updateRef, id, run))
		journal := RecreateJournal{RunID: run, OldID: id, OldName: name, BackupName: name + "-docklet-recreate-" + run, NewName: name, Phase: "renamed", WasRunning: true, UpdatedAt: stamp, ExpectedImageID: new(updateNewImage)}
		old := d.containers[id]
		old.Name = "/" + journal.BackupName
		old.State.Running = true
		d.containers[id] = old
		require.NoError(t, db.Create(&journal).Error)
	}
	return f
}

func (f *recoveryFixture) receipt(t *testing.T, name string) activity.RuntimeReceipt {
	t.Helper()
	var row activity.RuntimeReceipt
	require.NoError(t, f.db.First(&row, "run_id=?", f.runs[name]).Error)
	return row
}

func (f *recoveryFixture) journalPhase(t *testing.T, name string) string {
	t.Helper()
	var j RecreateJournal
	require.NoError(t, f.db.Table("container_recreate_journal").First(&j, "run_id=?", f.runs[name]).Error)
	return j.Phase
}

func (f *recoveryFixture) effects() int {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	return f.d.counts["rename"] + f.d.counts["start"] + f.d.counts["remove"] + f.d.counts["create"] + f.d.counts["pull"]
}

func (f *recoveryFixture) requireHeld(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		row := f.receipt(t, name)
		require.Nil(t, row.SettledAt, "%s stays unsettled, so its scope stays excluded", name)
		require.Equal(t, "unknown", *row.ExecutorState, "%s is held as unknown, not as progress", name)
		require.Equal(t, "renamed", f.journalPhase(t, name), "%s keeps its journal for the next start", name)
	}
	require.Zero(t, f.effects(), "no Docker effect without an observation")
}

// The next start with Docker reachable settles each held run once from its
// journal, without replaying the update.
func (f *recoveryFixture) requireRecoveredOnce(t *testing.T, names ...string) {
	t.Helper()
	f.fault.set("", 0)
	require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
	settledAt := map[string]time.Time{}
	for _, name := range names {
		row := f.receipt(t, name)
		require.NotNil(t, row.SettledAt, "%s settles on the next start", name)
		require.Equal(t, "failed", *row.ExecutorState)
		settledAt[name] = *row.SettledAt
		id := *row.TargetID
		require.Equal(t, "/"+name, f.d.containers[id].Name, "compensation restores the original name")
		require.True(t, f.d.containers[id].State.Running)
	}
	require.Zero(t, f.d.counts["pull"])
	require.Zero(t, f.d.counts["create"])
	require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
	for _, name := range names {
		require.True(t, settledAt[name].Equal(*f.receipt(t, name).SettledAt), "%s settles only once", name)
	}
}

// An unavailable or silent daemon no longer aborts startup: every pending
// journal is retried within the bound, then held unknown for the next start.
func TestRecreateRecoveryHoldsRunsWhileDockerIsUnavailable(t *testing.T) {
	for _, mode := range []string{"unavailable", "silent"} {
		t.Run(mode, func(t *testing.T) {
			withRecoveryTiming(t, 10*time.Second, 100*time.Millisecond, 10*time.Millisecond)
			f := newRecoveryFixture(t, "app", "worker")
			f.fault.set(mode, -1)
			require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store), "startup continues")
			require.GreaterOrEqual(t, f.fault.brokenInspects(), 2*recreateRecoveryTiming.attempts, "each attempt passes over both journals")
			f.requireHeld(t, "app", "worker")
			f.requireRecoveredOnce(t, "app", "worker")
		})
	}
}

// A daemon that answers again within the bound settles in the same start.
func TestRecreateRecoveryRetriesWithinTheStartupBound(t *testing.T) {
	withRecoveryTiming(t, 10*time.Second, 100*time.Millisecond, 10*time.Millisecond)
	f := newRecoveryFixture(t, "app")
	f.fault.set("unavailable", 1)
	require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
	row := f.receipt(t, "app")
	require.NotNil(t, row.SettledAt, "the second attempt recovers the journal")
	require.Equal(t, "/app", f.d.containers[updateOldID].Name)
}

// The whole recovery shares one budget; a daemon that holds every call cannot
// hold startup past it even when the per-call budget is longer.
func TestRecreateRecoveryEndsWithTheStartupBudget(t *testing.T) {
	withRecoveryTiming(t, 200*time.Millisecond, time.Hour, 10*time.Millisecond)
	f := newRecoveryFixture(t, "app", "worker")
	f.fault.set("silent", -1)
	done := make(chan error, 1)
	go func() { done <- f.rt.InitializeLocalStartup(t.Context(), f.store) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("startup recovery outlived its budget")
	}
	f.requireHeld(t, "app", "worker")
}

// A journal write failure is a store failure, not a Docker one: startup fails.
func TestRecreateRecoveryJournalWriteFailureStaysFatal(t *testing.T) {
	withRecoveryTiming(t, 10*time.Second, 100*time.Millisecond, 10*time.Millisecond)
	f := newRecoveryFixture(t, "app")
	require.NoError(t, f.db.Exec("CREATE TRIGGER recovery_journal_write_fails BEFORE UPDATE ON container_recreate_journal BEGIN SELECT RAISE(ABORT, 'synthetic journal write failure'); END").Error)
	require.Error(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
	require.Nil(t, f.receipt(t, "app").SettledAt)
}
