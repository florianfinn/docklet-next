// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const deployProgressCompose = "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n"

func deployProgressLabels() map[string]string {
	return map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"}
}

func TestRuntimeDeployProgressDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, operation := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
			t.Run(map[bool]string{false: "direct", true: "edge"}[edge]+"/"+string(operation), func(t *testing.T) {
				f := newRemoteIntegration(t, edge)
				f.agent.project("example-project", "example-project", deployProgressCompose)
				f.agent.http.setLabels(deployProgressLabels())
				entered, release := make(chan struct{}), make(chan struct{})
				startEntered, startRelease := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				var startOnce sync.Once
				unblockStart := func() { startOnce.Do(func() { close(startRelease) }) }
				defer unblockStart()
				defer unblock()
				f.agent.http.mu.Lock()
				f.agent.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
						close(startEntered)
						select {
						case <-startRelease:
						case <-r.Context().Done():
						}
						return false
					}
					if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/images/create") {
						return false
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, "{\"id\":\"abcdef012345\",\"status\":\"Downloading\",\"progressDetail\":{\"current\":40,\"total\":100}}\n")
					_, _ = io.WriteString(w, "{\"status\":\"Pull progress witness\"}\n")
					w.(http.Flusher).Flush()
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return true
					}
					_, _ = io.WriteString(w, "{\"id\":\"abcdef012345\",\"status\":\"Pull complete\"}\n")
					return true
				}
				f.agent.http.mu.Unlock()
				command := f.command(operation, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
				command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "missing"}
				if operation == activity.RuntimeDeployProject {
					command.Options.Deploy.PullPolicy = "always"
				}
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				defer func() { unblock(); unblockStart(); _, _ = f.agent.waitLocal(ref.RunID) }()
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("real pull effect barrier not reached")
				}
				// Query through the actual socket-backed sender while the agent is held.
				require.Eventually(t, func() bool {
					_, err := f.control.Refresh(t.Context(), ref)
					if err != nil {
						return false
					}
					detail, err := f.hubActivity.GetActivityDetail(t.Context(), f.store.EnvironmentID, ref.RunID, 100)
					if err != nil {
						return false
					}
					for _, line := range detail.Output {
						if strings.Contains(line.Text, "Pull progress witness") {
							return true
						}
					}
					return false
				}, 5*time.Second, 10*time.Millisecond, "hub must consume the real pull stream before checking its progress")
				detail, err := f.hubActivity.GetActivityDetail(t.Context(), f.store.EnvironmentID, ref.RunID, 100)
				require.NoError(t, err)
				require.Equal(t, activityDTO.StatusRunning, detail.Activity.Status)
				require.Equal(t, "pull", detail.Activity.Step, "real reporter must bind the pull phase to the hub run")
				require.NotNil(t, detail.Activity.Metadata["deployPull"], "real reporter must persist pull counts before settlement")
				raw, err := json.Marshal(detail.Activity.Metadata["deployPull"])
				require.NoError(t, err)
				var counts map[string]int64
				require.NoError(t, json.Unmarshal(raw, &counts))
				require.EqualValues(t, 40, counts["bytesDone"])
				require.EqualValues(t, 100, counts["bytesTotal"])
				require.EqualValues(t, 1, counts["layersTotal"])
				require.Equal(t, "pull", detail.StepTrace[0].Name)
				unblock()
				select {
				case <-startEntered:
				case <-time.After(5 * time.Second):
					t.Fatal("real deploy start barrier not reached")
				}
				require.Eventually(t, func() bool {
					_, err := f.control.Refresh(t.Context(), ref)
					if err != nil {
						return false
					}
					detail, err := f.hubActivity.GetActivityDetail(t.Context(), f.store.EnvironmentID, ref.RunID, 100)
					if err != nil {
						return false
					}
					raw, _ := json.Marshal(detail.Activity.Metadata["deployActivity"])
					var observation struct {
						Label    string
						Services int
					}
					_ = json.Unmarshal(raw, &observation)
					return detail.Activity.Step == "deploy" && detail.Activity.Status == activityDTO.StatusRunning && observation.Label == "start" && observation.Services == 1
				}, 4*time.Second, 20*time.Millisecond, "current Compose activity must reach the hub during deploy")
				unblockStart()
				_, err = f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				result, err := f.control.Wait(t.Context(), ref, nil)
				require.NoError(t, err)
				require.Equal(t, activityDTO.StatusSuccess, result.Status)
				detail, err = f.hubActivity.GetActivityDetail(t.Context(), f.store.EnvironmentID, ref.RunID, 100)
				require.NoError(t, err)
				require.Len(t, detail.StepTrace, 2)
				require.Equal(t, "pull", detail.StepTrace[0].Name)
				require.Equal(t, "deploy", detail.StepTrace[1].Name)
				require.Equal(t, activityDTO.StatusSuccess, detail.StepTrace[0].Status)
				require.Equal(t, activityDTO.StatusSuccess, detail.StepTrace[1].Status)
				var output []string
				for _, line := range detail.Output {
					output = append(output, line.Text)
				}
				require.Contains(t, strings.Join(output, "\n"), "Pull complete", "raw output stays available")
			})
		}
	}
}

func TestRuntimeDeployProgressFailureSemantics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		op     activity.RuntimeOperation
		policy string
		want   []string
		status activityDTO.Status
	}{
		{"fatal-pull", activity.RuntimeDeployProject, "always", []string{"pull"}, activityDTO.StatusFailed},
		{"nonfatal-preliminary-pull", activity.RuntimeRedeployProject, "never", []string{"pull", "deploy"}, activityDTO.StatusSuccess},
		{"fatal-prepare", activity.RuntimeDeployProject, "never", []string{"deploy"}, activityDTO.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", deployProgressCompose)
			f.http.setLabels(deployProgressLabels())
			f.http.mu.Lock()
			f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "/images/create") {
					http.Error(w, "synthetic pull failure", 500)
					return true
				}
				if tc.name == "fatal-prepare" && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
					http.NotFound(w, r)
					return true
				}
				return false
			}
			f.http.mu.Unlock()
			command := f.command(tc.op, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: tc.policy}
			admitted, err := f.runtime.Admit(t.Context(), command)
			require.NoError(t, err)
			final, err := f.waitLocal(admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, tc.status, final.Status)
			detail, err := f.activity.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
			require.NoError(t, err)
			var names []string
			for _, step := range detail.StepTrace {
				names = append(names, step.Name)
			}
			require.Equal(t, tc.want, names)
			require.Equal(t, tc.status, detail.StepTrace[len(detail.StepTrace)-1].Status)
			if tc.name == "nonfatal-preliminary-pull" {
				warnings := 0
				for _, message := range detail.Messages {
					if message.Step == "pull" && message.Level == "warning" {
						warnings++
					}
				}
				require.Equal(t, 1, warnings, "nonfatal pull must retain a warning")
				require.Equal(t, activityDTO.StatusSuccess, detail.StepTrace[0].Status)
			}
		})
	}
}

func TestRuntimeDeployProgressCachedSkipsPullAndStop(t *testing.T) {
	for _, policy := range []string{"missing", "never"} {
		t.Run(policy, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", deployProgressCompose)
			f.http.setLabels(deployProgressLabels())
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: policy}
			f.run(command)
			f.http.setLabels(runtimeCreatedLabels(t, f.http.snapshot()))
			before := len(f.http.snapshot())
			id := f.run(command)
			detail, err := f.activity.GetActivityDetail(t.Context(), "0", id, 100)
			require.NoError(t, err)
			require.Len(t, detail.StepTrace, 1, "cached deploy has no invented pull")
			require.Equal(t, "deploy", detail.StepTrace[0].Name)
			for _, event := range f.http.snapshot()[before:] {
				require.False(t, strings.HasSuffix(event.Path, "/stop") || strings.HasSuffix(event.Path, "/images/create"))
			}
		})
	}
}

type deployTailExecutor struct{ delegate activity.RuntimeExecutor }

func (e deployTailExecutor) ExecuteRuntime(ctx context.Context, command activity.RuntimeCommand, out io.Writer) error {
	if err := e.delegate.ExecuteRuntime(ctx, command, out); err != nil {
		return err
	}
	reporter := projects.DeployReporterFromContext(ctx)
	reporter.ObservePull("image", []byte(`{"id":"abcdef012345","status":"Downloading","progressDetail":{"current":1,"total":100}}`))
	reporter.ObservePull("image", []byte(`{"id":"abcdef012345","status":"Downloading","progressDetail":{"current":99,"total":100}}`))
	return nil
}

func TestRuntimeDeployProgressFinalDrainAndPersistenceFailure(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "drain", true: "persistence-failure"}[reject], func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", deployProgressCompose)
			f.http.setLabels(deployProgressLabels())
			var logs bytes.Buffer
			if reject {
				previous := slog.Default()
				slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
				defer slog.SetDefault(previous)
			}
			f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.store, Executor: deployTailExecutor{f.adapter}})
			if reject {
				require.NoError(t, f.db.Exec(`CREATE TRIGGER reject_deploy_metadata BEFORE UPDATE OF metadata ON activities WHEN NEW.metadata != OLD.metadata AND NEW.metadata LIKE '%deployPull%' BEGIN SELECT RAISE(ABORT, 'synthetic metadata refusal'); END`).Error)
			}
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "never"}
			id := f.run(command)
			var row activity.Activity
			require.NoError(t, f.db.First(&row, "id = ?", id).Error)
			require.Equal(t, activityDTO.StatusSuccess, row.Status)
			if reject {
				require.Contains(t, logs.String(), "deploy observation persistence failed", "persistence failure must be logged")
			}
			if !reject {
				raw, err := json.Marshal(row.Metadata["deployPull"])
				require.NoError(t, err)
				var pull map[string]int64
				require.NoError(t, json.Unmarshal(raw, &pull))
				require.EqualValues(t, 99, pull["bytesDone"], "pending observation must drain before worker settlement")
			}
		})
	}
}

func TestRuntimeDeployProgressInterleavedServiceFailure(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", deployProgressCompose+"  worker:\n    image: registry.example.com/worker:1\n    depends_on: [app]\n")
	f.http.setLabels(deployProgressLabels())
	appStarted := make(chan struct{})
	var started sync.Once
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create") && strings.Contains(r.URL.Query().Get("name"), "worker") {
			var body struct{ Labels map[string]string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.http.setSecondComposeContainer(body.Labels)
			f.http.mu.Lock()
			worker := f.http.extraContainers[runtimeSecondContainerID]
			worker.running = false
			f.http.extraContainers[runtimeSecondContainerID] = worker
			f.http.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": runtimeSecondContainerID, "Warnings": []string{}})
			return true
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeSecondContainerID+"/start") {
			select {
			case <-appStarted:
			default:
				t.Error("first service must start before second service fails")
			}
			http.Error(w, "synthetic second service failure", http.StatusInternalServerError)
			return true
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			started.Do(func() { close(appStarted) })
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "always"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	final, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusFailed, final.Status)
	select {
	case <-appStarted:
	default:
		t.Fatal("first service must have started")
	}
	detail, err := f.activity.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
	require.NoError(t, err)
	require.Len(t, detail.StepTrace, 2, "interleaved resources must not produce trace phases")
	require.Equal(t, "pull", detail.StepTrace[0].Name)
	require.Equal(t, activityDTO.StatusSuccess, detail.StepTrace[0].Status)
	require.Equal(t, "deploy", detail.StepTrace[1].Name)
	require.Equal(t, activityDTO.StatusFailed, detail.StepTrace[1].Status)
	require.NotNil(t, detail.Activity.Metadata["deployActivity"])
}

func TestRuntimeDeployProgressConcurrentCancellation(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("first-project", "first-project", strings.ReplaceAll(deployProgressCompose, "example_stack", "first_stack"))
	f.project("second-project", "second-project", strings.ReplaceAll(deployProgressCompose, "app:1", "app:2"))
	f.http.setLabels(deployProgressLabels())
	releases := []chan struct{}{make(chan struct{}), make(chan struct{})}
	entered := []chan struct{}{make(chan struct{}), make(chan struct{})}
	var once [2]sync.Once
	unblock := func(i int) { once[i].Do(func() { close(releases[i]) }) }
	defer unblock(0)
	defer unblock(1)
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/images/create") {
			return false
		}
		index := 0
		if strings.Contains(r.URL.Query().Get("fromImage"), ":2") || r.URL.Query().Get("tag") == "2" {
			index = 1
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "abcdef012345", "status": "Downloading", "progressDetail": map[string]int{"current": 10 + index, "total": 100}})
		w.(http.Flusher).Flush()
		close(entered[index])
		select {
		case <-releases[index]:
		case <-r.Context().Done():
			return true
		}
		_, _ = io.WriteString(w, "{\"id\":\"abcdef012345\",\"status\":\"Pull complete\"}\n")
		return true
	}
	f.http.mu.Unlock()
	ids := make([]string, 2)
	for i, project := range []string{"first-project", "second-project"} {
		command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: project})
		command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "always"}
		admitted, err := f.runtime.Admit(t.Context(), command)
		require.NoError(t, err)
		ids[i] = admitted.ActivityID
		defer func(i int) { unblock(i); _, _ = f.waitLocal(ids[i]) }(i)
	}
	for _, barrier := range entered {
		select {
		case <-barrier:
		case <-time.After(5 * time.Second):
			t.Fatal("both runs must reach their pull effect")
		}
	}
	for i, id := range ids {
		require.Eventually(t, func() bool {
			detail, err := f.activity.GetActivityDetail(t.Context(), "0", id, 100)
			if err != nil {
				return false
			}
			raw, _ := json.Marshal(detail.Activity.Metadata["deployPull"])
			var pull map[string]int64
			_ = json.Unmarshal(raw, &pull)
			return detail.Activity.Step == "pull" && pull["bytesDone"] == int64(10+i)
		}, 3*time.Second, 10*time.Millisecond, "progress must bind to each concurrent run ID")
	}
	_, err := f.runtime.Cancel(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: ids[0]}, f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "first-project"}).Actor)
	require.NoError(t, err)
	first, err := f.waitLocal(ids[0])
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusCancelled, first.Status)
	second, err := f.activity.GetActivityDetail(t.Context(), "0", ids[1], 100)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusRunning, second.Activity.Status)
	require.Equal(t, "pull", second.Activity.Step)
	unblock(1)
	final, err := f.waitLocal(ids[1])
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, final.Status)
}

func TestRuntimeDeployProgressRestartInterruptsObservation(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", deployProgressCompose)
	storeOnly := activity.NewRuntimeService(f.activity, f.adapter)
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	admitted, err := storeOnly.Admit(t.Context(), command)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&activity.Activity{}).Where("id = ?", admitted.ActivityID).Updates(map[string]any{"step": "pull", "metadata": database.JSON{"deployPull": map[string]any{"v": 1, "images": 1, "layersDone": 0, "layersTotal": 1, "bytesDone": 40, "bytesTotal": 100}}}).Error)
	restarted := activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.store, Executor: deployNoReplayExecutor{t}})
	require.NoError(t, restarted.InitializeLocalStartup(t.Context(), f.store))
	detail, err := f.activity.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusFailed, detail.Activity.Status)
	require.Equal(t, "runtime.interrupted", detail.Activity.MessageCode)
	require.NotNil(t, detail.Activity.EndedAt, "restart must not leave stale progress on a running row")
}

func TestRuntimeDeployProgressDrainBarrier(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", deployProgressCompose)
	f.http.setLabels(deployProgressLabels())
	f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.store, Executor: deployTailExecutor{f.adapter}})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	require.NoError(t, f.db.Callback().Update().Before("gorm:update").Register("test:deploy-drain", func(tx *gorm.DB) {
		if tx.Statement.Table != "activities" {
			return
		}
		raw, _ := json.Marshal(tx.Statement.Dest)
		if strings.Contains(string(raw), `"bytesDone":99`) {
			select {
			case <-entered:
				return
			default:
				close(entered)
			}
			<-release
		}
	}))
	defer f.db.Callback().Update().Remove("test:deploy-drain")
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	defer func() { unblock(); _, _ = f.waitLocal(admitted.ActivityID) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reporter final persistence did not reach drain barrier")
	}
	reader, err := sqlitestore.Open(t.Context(), f.dbPath, true, true)
	require.NoError(t, err)
	defer reader.Close()
	var terminal, settled int
	require.NoError(t, reader.QueryRowContext(t.Context(), "SELECT count(*) FROM activities WHERE id = ? AND status IN ('success','failed','cancelled')", admitted.ActivityID).Scan(&terminal))
	require.Zero(t, terminal, "reporter persistence must join before terminal activity")
	require.NoError(t, reader.QueryRowContext(t.Context(), "SELECT count(*) FROM runtime_receipts WHERE run_id = ? AND settled_at IS NOT NULL", admitted.ActivityID).Scan(&settled))
	require.Zero(t, settled, "reporter persistence must join before owner settlement")
	unblock()
	final, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, final.Status)
}
