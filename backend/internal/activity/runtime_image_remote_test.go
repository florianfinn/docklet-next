// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Real Direct/Edge lifecycle execution, query and lost-ack receipt recovery.

package activity_test

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRuntimeImageActualDirectEdgeUpdateRestoreAndLostAck(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
			fw := francistest.New(t)
			gate := runs.NewAdmission(fw.Service(), t.Name())
			require.NoError(t, gate.Register(fw))
			francistest.Start(t, fw)
			f.agent.images.SetImageAdmission(gate, nil)
			oldID := "sha256:" + strings.Repeat("b", 64)
			newID := "sha256:" + strings.Repeat("e", 64)
			digest := "sha256:" + strings.Repeat("f", 64)
			oldRef := "registry.example.com/app:1.0.0"
			newRef := "registry.example.com/app:1.1.0"
			tags := map[string]string{oldID: oldID, newID: newID, oldRef: oldID}
			reference := oldRef
			var mu sync.Mutex
			f.agent.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
			f.agent.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				path := r.URL.Path
				if i := strings.Index(path[1:], "/"); i >= 0 {
					path = path[i+1:]
				}
				w.Header().Set("Content-Type", "application/json")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
					ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json"))
					id, ok := tags[ref]
					if !ok {
						w.WriteHeader(404)
						_, _ = w.Write([]byte(`{"message":"not found"}`))
						return true
					}
					all := []string{}
					for tag, value := range tags {
						if value == id && !strings.HasPrefix(tag, "sha256:") {
							all = append(all, tag)
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoTags": all, "Config": map[string]any{}})
					return true
				case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/tag"):
					ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/tag"))
					target := r.URL.Query().Get("repo") + ":" + r.URL.Query().Get("tag")
					tags[target] = tags[ref]
					w.WriteHeader(201)
					return true
				case path == "/images/create":
					tags["registry.example.com/app@"+digest] = newID
					_, _ = w.Write([]byte(`{"status":"downloaded"}`))
					return true
				case strings.HasPrefix(path, "/images/") && r.Method == http.MethodDelete:
					ref, _ := url.PathUnescape(strings.TrimPrefix(path, "/images/"))
					delete(tags, ref)
					_, _ = w.Write([]byte(`[]`))
					return true
				case path == "/containers/create":
					f.agent.http.mu.Lock()
					body := f.agent.http.events[len(f.agent.http.events)-1].Body
					f.agent.http.mu.Unlock()
					var conf struct{ Image string }
					require.NoError(t, json.Unmarshal([]byte(body), &conf))
					reference = conf.Image
					f.agent.http.mu.Lock()
					if f.agent.http.activeID == runtimeReplacementContainerID {
						f.agent.http.composeContainer = false
						f.agent.http.activeID = ""
					}
					f.agent.http.mu.Unlock()
					return false
				case path == "/containers/json" && r.URL.Query().Get("filters") == "":
					f.agent.http.mu.Lock()
					present, running, id, labels := f.agent.http.composeContainer, f.agent.http.running, f.agent.http.activeID, f.agent.http.labels
					f.agent.http.mu.Unlock()
					rows := []any{}
					if present {
						state := "exited"
						if running {
							state = "running"
						}
						rows = append(rows, map[string]any{"Id": id, "Image": reference, "ImageID": tags[reference], "State": state, "Labels": labels})
					}
					_ = json.NewEncoder(w).Encode(rows)
					return true
				case path != "/containers/json" && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
					id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
					f.agent.http.mu.Lock()
					running, labels := f.agent.http.running, f.agent.http.labels
					f.agent.http.mu.Unlock()
					state := "exited"
					if running {
						state = "running"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/example_stack-app-1", "Image": tags[reference], "Config": map[string]any{"Image": reference, "Labels": labels}, "State": map[string]any{"Status": state, "Running": running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
					return true
				}
				return false
			}
			target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
			source := "name: example_stack\nservices:\n  app:\n    image: " + oldRef + "\n"
			path := f.agent.project(target.ID, target.ID, source)
			deploy := f.agent.command(activity.RuntimeDeployProject, target)
			deploy.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "never"}
			f.agent.run(deploy)
			baseline, err := f.agent.projects.ImageBaselineStatus(t.Context(), target.ID, f.store.RegistrationID)
			require.NoError(t, err, "baseline code=%s events=%s", imageDTO.Neutral(err, "unknown", 500).Problem.Code, f.agent.debugEvents())
			require.True(t, baseline.Available)
			var row life.Baseline
			require.NoError(t, f.agent.db.First(&row, "target_id = ?", target.ID).Error)
			intent := &dto.LifecycleIntent{Request: dto.LifecycleRequest{ExpectedRevision: baseline.SavedRevision}, TargetIdentity: row.TargetIdentity, SavedRevision: baseline.SavedRevision, ObservedRevision: baseline.ObservedRevision, BaselineRevision: row.Revision, Selections: []dto.ImageSelection{{Service: "app", SourceReference: oldRef, Reference: newRef, Digest: digest, ImageID: newID, Platform: "linux/amd64", Kind: "version"}}}
			run := func(op activity.RuntimeOperation, intent *dto.LifecycleIntent) activityDTO.RuntimeSnapshot {
				command := f.command(op, target)
				command.Options.Lifecycle = intent
				if op == activity.RuntimeUpdateProject {
					store := policy.Store{DB: f.db.DB}
					one, err := store.Policy(t.Context(), command.Store.EnvironmentID, command.Store.RegistrationID, target.ID)
					require.NoError(t, err)
					one.Mode, one.ActorID, one.TargetIdentity = "apply", command.Actor.ID, intent.TargetIdentity
					require.NoError(t, store.SavePolicy(t.Context(), one, 0))
					one.Revision = 1
					child, fresh, err := store.PrepareChild(t.Context(), one, "example-policy-job")
					require.NoError(t, err)
					require.True(t, fresh)
					intent.Policy = &dto.PolicyBinding{PolicyID: one.ID, Revision: 1, JobRunID: child.JobRunID, ChildID: child.ID}
					command.ProposedRunID, command.Key = child.ID, child.ID
				}
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				if intent.Policy != nil {
					require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", intent.Policy.PolicyID).Updates(map[string]any{"mode": "off", "revision": 2}).Error)
				}
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				agent, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.Equal(t, activityDTO.StatusSuccess, agent.Status, f.agent.debugEvents())
				out, err := f.control.Wait(ctx, ref, nil)
				require.NoError(t, err)
				require.Equal(t, activityDTO.StatusSuccess, out.Status)
				require.NotNil(t, out.ImageLifecycle)
				require.True(t, out.ImageLifecycle.Verified)
				before := f.executeCalls.Load()
				_, err = f.control.Start(ctx, ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				require.Equal(t, before, f.executeCalls.Load())
				return out
			}
			f.loseAck.Store(true)
			updated := run(activity.RuntimeUpdateProject, intent)
			body, err := os.ReadFile(filepath.Join(path, "compose.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(body), newRef)
			baseline, err = f.agent.projects.ImageBaselineStatus(t.Context(), target.ID, f.store.RegistrationID)
			require.NoError(t, err)
			meta, err := f.sender.Resolve(t.Context(), activityDTO.RuntimeTargetRequest{Registration: activityDTO.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: target, Lifecycle: &dto.LifecycleRequest{PointID: updated.ImageLifecycle.RestorePointID, ExpectedRevision: baseline.SavedRevision, ReplaceConfiguration: true}})
			require.NoError(t, err)
			require.NotNil(t, meta.Lifecycle)
			run(activity.RuntimeRestoreProject, meta.Lifecycle)
			body, err = os.ReadFile(filepath.Join(path, "compose.yaml"))
			require.NoError(t, err)
			require.Equal(t, source, string(body))
			mu.Lock()
			require.Equal(t, oldID, tags[oldRef])
			mu.Unlock()
			require.EqualValues(t, 2, f.executeCalls.Load())
		})
	}
}
