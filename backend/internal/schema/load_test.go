// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Optional repeatable fifteen-minute service-layer acceptance benchmark.

package schema

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	francistesting "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Run with -run '^$' -bench '^BenchmarkSQLiteConcurrentLoad$' -benchtime=1x -timeout=20m.
func BenchmarkSQLiteConcurrentLoad(b *testing.B) {
	const duration = 15 * time.Minute
	path := filepath.Join(b.TempDir(), "hub.db")
	url := sqlitestore.URL(path)
	ctx := context.Background()
	db, err := database.Initialize(b.Context(), url, database.MigrationOptions{})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	svc := activity.NewActivityService(db, nil)
	envs := environment.NewEnvironmentService(db, nil, nil, nil, nil, nil)
	values := kv.NewKVService(db)
	actors := francistesting.New(b, url)
	if err = actors.RegisterActor("load", func(string, *actor.Service) actor.Actor { return &sqliteLoadActor{service: actors.Service()} }); err != nil {
		b.Fatal(err)
	}
	francistesting.Start(b, actors)
	diagnostics, err := actors.Store()
	if err != nil {
		b.Fatal(err)
	}
	var runs [4]activity.GateAnswer
	for i := range 4 {
		record := environment.Environment{Name: fmt.Sprintf("agent-%d", i), ApiUrl: "https://agent.example.com", Status: "offline"}
		record.ID = fmt.Sprintf("env-%d", i)
		if err = db.Create(&record).Error; err != nil {
			b.Fatal(err)
		}
		runs[i], err = svc.StartGatedActivity(ctx, activity.GateRequest{EnvironmentID: record.ID, Operation: "load", Scope: "output", Type: activitytypes.TypeProjectDeploy})
		if err != nil {
			b.Fatal(err)
		}
	}
	pool, _ := db.SQLDB()
	var sqliteVersion string
	var pageSize int
	_ = pool.QueryRow("SELECT sqlite_version()").Scan(&sqliteVersion)
	_ = pool.QueryRow("PRAGMA page_size").Scan(&pageSize)
	fileSize := func(p string) int64 {
		if info, err := os.Stat(p); err == nil {
			return info.Size()
		}
		return 0
	}
	initial := fileSize(path)
	sidecar := sqlitestore.FrancisPath(path)
	initialSidecar := fileSize(sidecar)
	var mu sync.Mutex
	var latencies []time.Duration
	var failures []error
	var busy, locked atomic.Int64
	stop := make(chan struct{})
	var stopOnce sync.Once
	stopAll := func() { stopOnce.Do(func() { close(stop) }) }
	record := func(start time.Time, err error) {
		mu.Lock()
		defer mu.Unlock()
		latencies = append(latencies, time.Since(start))
		if err != nil {
			failures = append(failures, err)
			if strings.Contains(err.Error(), "SQLITE_BUSY") {
				busy.Add(1)
			}
			if strings.Contains(err.Error(), "SQLITE_LOCKED") {
				locked.Add(1)
			}
			stopAll()
		}
	}
	var wg sync.WaitGroup
	repeat := func(interval time.Duration, call func() error, write bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					start := time.Now()
					err := call()
					if write {
						record(start, err)
					} else if err != nil {
						record(start, err)
					}
				}
			}
		}()
	}
	var outputs [4]atomic.Int64
	var interactive [3]atomic.Int64
	var heartbeats [4]atomic.Int64
	var receipts atomic.Int64
	var reads atomic.Int64
	var peaks [2]atomic.Int64
	started := time.Now()
	timer := time.AfterFunc(duration, stopAll)
	defer timer.Stop()
	emit := func(i int, target int64) error {
		current := outputs[i].Load()
		count := target - current
		if count > 32 {
			return fmt.Errorf("output backlog exceeded one bounded batch")
		}
		if count <= 0 {
			return nil
		}
		batch := make([]activity.AppendActivityMessageRequest, 0, count)
		for offset := range count {
			line := fmt.Sprintf("line-%d-%d", i, current+offset)
			batch = append(batch, activity.AppendActivityMessageRequest{Message: line, Output: &activitytypes.OutputLine{Timestamp: time.Now().UTC(), Stream: "stdout", Text: line}})
		}
		callStart := time.Now()
		_, err := svc.AppendMessages(runs[i].WorkContext, runs[i].Run.ID, batch)
		record(callStart, err)
		if err == nil {
			outputs[i].Add(count)
		}
		return err
	}
	interact := func(i int, target int64) error {
		for interactive[i].Load() < target {
			callStart := time.Now()
			_, err := values.IncrementInt64(ctx, fmt.Sprintf("user-%d", i), 1)
			record(callStart, err)
			if err != nil {
				return err
			}
			interactive[i].Add(1)
		}
		return nil
	}
	heartbeat := func(i int) error {
		err := envs.UpdateEnvironmentHeartbeat(ctx, fmt.Sprintf("env-%d", i))
		if err == nil {
			heartbeats[i].Add(1)
		}
		return err
	}
	for i := range 4 {
		repeat(100*time.Millisecond, func() error {
			return emit(i, min(int64(duration/(100*time.Millisecond)), int64(time.Since(started)/(100*time.Millisecond))))
		}, false)
		repeat(5*time.Second, func() error { return heartbeat(i) }, true)
	}
	for i := range 3 {
		repeat(time.Second, func() error {
			return interact(i, min(int64(duration/time.Second), int64(time.Since(started)/time.Second)))
		}, false)
	}

	repeat(time.Second, func() error {
		for i := range 4 {
			answer, err := svc.StartGatedActivity(ctx, activity.GateRequest{EnvironmentID: fmt.Sprintf("env-%d", i), Operation: "load", Scope: "output", Type: activitytypes.TypeProjectDeploy})
			if err != nil {
				return err
			}
			if answer.Code != "run.joined" || answer.WorkContext != nil {
				return fmt.Errorf("duplicate worker on scope %d", i)
			}
		}
		return nil
	}, true)
	repeat(time.Second, func() error {
		stamp := time.Now().UTC()
		err := svc.SyncJobRun(ctx, scheduler.Run{ID: "load-job", ActivityID: "load-job-activity", JobID: "example-job", EnvironmentID: "0", Status: scheduler.Succeeded, CreatedAt: started, UpdatedAt: stamp}, "Load job")
		if err == nil {
			receipts.Add(1)
		}
		return err
	}, true)
	repeat(time.Second, func() error {
		_, err := actors.Service().Invoke(ctx, "load", "load", "tick", nil)
		return err
	}, true)
	repeat(200*time.Millisecond, func() error {
		_, err := svc.GetActivityDetail(ctx, "env-0", runs[0].Run.ID, 20)
		if err != nil {
			return err
		}
		var count int64
		if err = diagnostics.Table("arcane_francis_hosts").Count(&count).Error; err == nil {
			reads.Add(1)
		}
		return err
	}, false)
	repeat(time.Second, func() error {
		for i, p := range []string{path, sidecar} {
			size := fileSize(p + "-wal")
			if size > peaks[i].Load() {
				peaks[i].Store(size)
			}
		}
		return nil
	}, false)
	<-stop
	wg.Wait()
	if len(failures) > 0 {
		b.Fatalf("workload error after %s: %v", time.Since(started), failures)
	}
	for i := range 4 {
		if err = emit(i, int64(duration/(100*time.Millisecond))); err != nil {
			b.Fatal(err)
		}
		for heartbeats[i].Load() < int64(duration/(5*time.Second)) {
			if err = heartbeat(i); err != nil {
				b.Fatal(err)
			}
		}
	}
	for i := range 3 {
		if err = interact(i, int64(duration/time.Second)); err != nil {
			b.Fatal(err)
		}
	}
	elapsed := time.Since(started)
	for i := range 4 {
		if _, err = svc.CompleteActivity(runs[i].WorkContext, runs[i].Run.ID, activitytypes.StatusSuccess, "finished", nil); err != nil {
			b.Fatal(err)
		}
		var count int64
		if err = db.Model(&activity.ActivityMessage{}).Where("activity_id=?", runs[i].Run.ID).Count(&count).Error; err != nil {
			b.Fatal(err)
		}
		if count != outputs[i].Load()+1 {
			b.Fatalf("missing/duplicate messages: %d != %d", count, outputs[i].Load()+1)
		}
	}
	for i := range 3 {
		value, err := values.GetInt64(ctx, fmt.Sprintf("user-%d", i), 0)
		if err != nil || value != interactive[i].Load() {
			b.Fatalf("interactive count: %d %v", value, err)
		}
	}
	var active, negative, receiptRows int64
	db.Model(&activity.Activity{}).Where("status IN ?", []string{"running", "queued"}).Count(&active)
	db.Model(&activity.Activity{}).Where("duration_ms < 0").Count(&negative)
	db.Model(&activity.ActivitySyncReceipt{}).Count(&receiptRows)
	if active != 0 || negative != 0 || receiptRows != 1 {
		b.Fatalf("run constraints: active=%d negative=%d receipts=%d", active, negative, receiptRows)
	}
	var jobMessages int64
	if err = db.Model(&activity.ActivityMessage{}).Where("activity_id=?", "load-job-activity").Count(&jobMessages).Error; err != nil {
		b.Fatal(err)
	}
	if jobMessages != receipts.Load() {
		b.Fatalf("coordinator message count: %d != %d", jobMessages, receipts.Load())
	}
	if err = actors.Stop(ctx); err != nil {
		b.Fatal(err)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[(len(latencies)-1)*95/100]
	p99 := latencies[(len(latencies)-1)*99/100]

	for _, p := range []string{path, sidecar} {
		check, err := sqlitestore.Open(ctx, p, p == path, false)
		if err != nil {
			b.Fatal(err)
		}
		var blocked, log, checkpointed int
		if err = check.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&blocked, &log, &checkpointed); err != nil || blocked != 0 || log != checkpointed {
			b.Fatalf("passive checkpoint %d %d %d: %v", blocked, log, checkpointed, err)
		}
		if err = check.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&blocked, &log, &checkpointed); err != nil || blocked != 0 || log != 0 {
			b.Fatalf("truncate checkpoint: %v", err)
		}
		_ = check.Close()
	}
	b.Logf("duration=%s platform=%s/%s CPUs=%d SQLite=%s driver=modernc-v1.60.1 page_size=%d samples=%d p95=%s p99=%s busy=%d locked=%d WAL_main_peak=%d WAL_francis_peak=%d main_initial=%d main_final=%d francis_initial=%d francis_final=%d heartbeats=%d reads=%d job_syncs=%d outputs=[%d,%d,%d,%d] interactive=[%d,%d,%d]", elapsed, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), sqliteVersion, pageSize, len(latencies), p95, p99, busy.Load(), locked.Load(), peaks[0].Load(), peaks[1].Load(), initial, fileSize(path), initialSidecar, fileSize(sidecar), heartbeats[0].Load()+heartbeats[1].Load()+heartbeats[2].Load()+heartbeats[3].Load(), reads.Load(), receipts.Load(), outputs[0].Load(), outputs[1].Load(), outputs[2].Load(), outputs[3].Load(), interactive[0].Load(), interactive[1].Load(), interactive[2].Load())
	if p95 > 250*time.Millisecond || busy.Load() != 0 || locked.Load() != 0 || peaks[0].Load() > 64<<20 || peaks[1].Load() > 64<<20 {
		b.Fatal("candidate exceeds SQLite acceptance thresholds")
	}
}

// sqliteLoadActor exercises actor lookup, activation and durable state on the writer.
type sqliteLoadActor struct{ service *actor.Service }

func (a *sqliteLoadActor) Invoke(ctx context.Context, _ string, _ actor.Envelope) (any, error) {
	return nil, a.service.SetState(ctx, "load", "load", map[string]int64{"tick": time.Now().UnixMilli()}, nil)
}
