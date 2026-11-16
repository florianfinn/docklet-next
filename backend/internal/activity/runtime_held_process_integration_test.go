// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Agent process kill/reopen and reader-independent health/reconnect recovery on real Direct and Edge carriers.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	a "github.com/florianfinn/docklet-next/types/activity"
	backup "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

type heldAgentProcessState struct {
	Path, EndpointFile, EnteredFile, SettledFile, Token, Mode string
	Store                                                     activity.RuntimeStoreBinding
}

type heldAgentExecutor struct{ state heldAgentProcessState }

func (e heldAgentExecutor) ExecuteRuntime(ctx context.Context, command activity.RuntimeCommand, _ io.Writer) error {
	if err := os.WriteFile(e.state.EnteredFile, []byte(command.ProposedRunID), 0600); err != nil {
		return err
	}
	if e.state.Mode == "interrupted" {
		<-ctx.Done()
		return ctx.Err()
	}
	dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: command.ProposedRunID, Code: "update.deploy-failed", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "not-needed"}})
	return errors.New("synthetic executor failure")
}
func (e heldAgentExecutor) RuntimeSettled(context.Context) {
	_ = os.WriteFile(e.state.SettledFile, []byte("settled"), 0600)
}

func TestRuntimeRecoveryAgentProcessChild(t *testing.T) {
	raw := os.Getenv("RUNTIME_HELD_AGENT_PROCESS")
	if raw == "" {
		return
	}
	var state heldAgentProcessState
	require.NoError(t, json.Unmarshal([]byte(raw), &state))
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	runtime := activity.NewRuntimeService(activity.NewActivityService(db, nil), remoteProcessResolver{})
	require.NoError(t, runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: state.Store, Executor: heldAgentExecutor{state}}))
	require.NoError(t, runtime.InitializeLocalStartup(t.Context(), state.Store))
	receiver, err := activity.NewRuntimeReceiver(runtime)
	require.NoError(t, err)
	handler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: state.Token}, time.Minute)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	require.NoError(t, os.WriteFile(state.EndpointFile, []byte(server.URL), 0600))
	<-t.Context().Done()
}

type heldHealthScheduler struct{ job scheduler.Job }

func (s *heldHealthScheduler) AddJob(_ context.Context, job scheduler.Job) error {
	s.job = job
	return nil
}
func (*heldHealthScheduler) RemoveJob(context.Context, string) {}
func (*heldHealthScheduler) HasJob(string) bool                { return false }
func (*heldHealthScheduler) Submit(context.Context, scheduler.Request) (scheduler.Run, error) {
	return scheduler.Run{}, nil
}

func TestRuntimeHeldRecoveryHealthTickDirectEdgeAgentRestart(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	for _, isEdge := range []bool{false, true} {
		for _, scenario := range []string{"interrupted", "failed", "reconnect-interrupted", "reconnect-failed"} {
			if !isEdge && strings.HasPrefix(scenario, "reconnect-") {
				continue
			}
			mode := strings.TrimPrefix(scenario, "reconnect-")
			t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge]+"/"+scenario, func(t *testing.T) {
				f := newRemoteIntegration(t, isEdge)
				f.agent.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
				command := f.command(activity.RuntimeUpdateProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
				command.Options.Lifecycle = observedRemoteIntent()
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				// The child opens the actual migrated agent store after the fixture releases it.
				require.NoError(t, f.agent.db.Close())
				dir := t.TempDir()
				state := heldAgentProcessState{Path: f.agent.dbPath, EndpointFile: filepath.Join(dir, "endpoint"), EnteredFile: filepath.Join(dir, "entered"), SettledFile: filepath.Join(dir, "settled"), Token: f.token, Mode: mode, Store: f.agent.store}
				payload, err := json.Marshal(state)
				require.NoError(t, err)
				child := exec.Command(os.Args[0], "-test.run=^TestRuntimeRecoveryAgentProcessChild$")
				child.Env = append(os.Environ(), "RUNTIME_HELD_AGENT_PROCESS="+string(payload))
				var log bytes.Buffer
				child.Stdout, child.Stderr = &log, &log
				require.NoError(t, child.Start())
				joined := false
				t.Cleanup(func() {
					if !joined {
						_ = child.Process.Kill()
						_ = child.Wait()
					}
				})
				waitFile := func(path string) {
					t.Helper()
					require.Eventually(t, func() bool { _, err := os.Stat(path); return err == nil }, 10*time.Second, 10*time.Millisecond)
				}
				waitFile(state.EndpointFile)
				endpoint, err := os.ReadFile(state.EndpointFile)
				require.NoError(t, err)
				target, err := url.Parse(string(endpoint))
				require.NoError(t, err)
				f.handlerMu.Lock()
				f.receiverHandler = httputil.NewSingleHostReverseProxy(target)
				f.handlerMu.Unlock()
				f.dropTerminal.Store(true)
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				waitFile(state.EnteredFile)
				if mode == "failed" {
					waitFile(state.SettledFile)
				}
				require.NoError(t, child.Process.Kill())
				require.Error(t, child.Wait(), "the executor process must be killed, not gracefully stopped")
				joined = true
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				require.NoError(t, activity.WaitRuntimeRemoteIdleForTest(ctx, f.control, ref.RunID))
				var held activity.RuntimeReceipt
				require.NoError(t, f.db.First(&held, "run_id = ?", ref.RunID).Error)
				require.Equal(t, new("transport"), held.UnknownReason)
				require.Nil(t, held.SettledAt)
				// Startup gets a fresh pool, runtime, receiver and no permission to replay effects.
				agentDB, err := database.InitializeHub(t.Context(), sqlitestore.URL(f.agent.dbPath), database.MigrationOptions{})
				require.NoError(t, err)
				defer agentDB.Close()
				runtime := activity.NewRuntimeService(activity.NewActivityService(agentDB, nil), remoteProcessResolver{})
				require.NoError(t, runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.agent.store, Executor: deployNoReplayExecutor{t}}))
				require.NoError(t, runtime.InitializeLocalStartup(t.Context(), f.agent.store))
				receiver, err := activity.NewRuntimeReceiver(runtime)
				require.NoError(t, err)
				handler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: f.token}, time.Minute)
				require.NoError(t, err)
				f.handlerMu.Lock()
				f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.URL.Path == "/api/app-version":
						_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
					case strings.HasSuffix(r.URL.Path, "/volumes/backups/targets"):
						var set backup.TargetSet
						require.NoError(t, json.NewDecoder(r.Body).Decode(&set))
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": backup.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: set.ContentHash}})
					case r.URL.Path == "/api/health", strings.HasSuffix(r.URL.Path, "/system/health"), strings.HasSuffix(r.URL.Path, "/container-registries/sync"):
						_, _ = w.Write([]byte(`{"success":true}`))
					default:
						handler.ServeHTTP(w, r)
					}
				})
				f.handlerMu.Unlock()
				f.dropTerminal.Store(false)
				edge.SetDefaultRegistry(f.tunnels)
				t.Cleanup(func() { edge.ClearDefaultRegistry(f.tunnels) })
				settingsService, err := settings.NewSettingsService(t.Context(), f.db)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
				require.NoError(t, f.db.Create(&apikey.ApiKey{BaseModel: database.BaseModel{ID: "example-key"}, Name: "Example", KeyHash: "example-hash", KeyPrefix: "example"}).Error)
				environments := environment.NewEnvironmentService(f.db, http.DefaultClient, nil, nil, settingsService, nil)
				require.NoError(t, f.db.Create(&environment.Environment{ID: f.store.EnvironmentID, Name: "Example", ApiUrl: f.endpoint, IsEdge: isEdge, Enabled: true, Status: "offline", AccessToken: &f.token, ApiKeyID: new("example-key")}).Error)
				environments.SetRuntimeRecovery(func(ctx context.Context, id string) error {
					identity, err := f.sender.Register(ctx, a.RuntimeRegistration{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID})
					if err != nil {
						return err
					}
					store := f.store
					store.ExecutorGeneration = identity.ExecutorGeneration
					return f.control.RecoverHeldTarget(ctx, store)
				})
				framework := francistest.New(t)
				gate := runs.NewAdmission(framework.Service(), t.Name())
				require.NoError(t, gate.Register(framework))
				francistest.Start(t, framework)
				scheduler := &heldHealthScheduler{}
				require.NoError(t, environments.SetScheduler(t.Context(), scheduler, gate))
				environments.RegisterHealthJobsOnStartup(t.Context())
				require.NotNil(t, scheduler.job)
				if strings.HasPrefix(scenario, "reconnect-") {
					connected := make(chan error, 1)
					f.handlerMu.Lock()
					f.onConnect = func(ctx context.Context) {
						go func() {
							err := environments.CheckReconnectedAgent(ctx, f.store.EnvironmentID)
							select {
							case connected <- err:
							default:
							}
						}()
					}
					f.handlerMu.Unlock()
					f.tunnels.Unregister(f.store.EnvironmentID)
					select {
					case err := <-connected:
						require.NoError(t, err)
					case <-ctx.Done():
						t.Fatal("real Edge client did not reconnect")
					}
				} else {
					_, err := scheduler.job.Run(ctx)
					require.NoError(t, err)
				}
				require.NoError(t, activity.WaitRuntimeRemoteIdleForTest(ctx, f.control, ref.RunID))
				var terminal activity.RuntimeReceipt
				require.NoError(t, f.db.First(&terminal, "run_id = ?", ref.RunID).Error)
				require.NotNil(t, terminal.SettledAt, "a successful health tick must settle without an Activity reader")
				require.Equal(t, new("runtime."+mode), terminal.ExecutorResultCode)
				for range 3 {
					_, err := scheduler.job.Run(ctx)
					require.NoError(t, err)
				}
				require.NoError(t, activity.WaitRuntimeRemoteIdleForTest(ctx, f.control, ref.RunID))
				var notices []notificationoutbox.Row
				require.NoError(t, f.db.Where("dedup_key = ?", "deploy:"+f.store.EnvironmentID+":"+ref.RunID).Find(&notices).Error)
				wantNotices := 0
				if mode == "failed" {
					wantNotices = 1
				}
				require.Len(t, notices, wantNotices)
				require.EqualValues(t, 1, f.executeCalls.Load(), "returning host never receives Execute again")
			})
		}
	}
}
