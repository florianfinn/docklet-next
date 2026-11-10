// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func updateStamp(t *testing.T) {
	t.Helper()
	oldVersion, oldRepo := config.Version, config.ReleaseRepository
	config.Version = "1.0.1"
	config.ReleaseRepository = "example.com/docklet"
	t.Cleanup(func() { config.Version = oldVersion; config.ReleaseRepository = oldRepo })
}
func configureUpdateFeed(t *testing.T, f *updateTransportFixture) {
	t.Helper()
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.1","published_at":"2026-11-01T12:00:00Z"}]`))
	}))
	t.Cleanup(feed.Close)
	f.hub.versions = version.NewVersionService(feed.Client(), false, feed.URL, "1.0.1", "example", nil, nil)
	f.hub.resolve = func(_ context.Context, repo, version string, _ http.RoundTripper) (string, error) {
		require.Equal(t, repository("agent"), repo)
		require.Equal(t, "1.0.1", version)
		return repo + ":" + version + "@sha256:" + testDigest, nil
	}
}
func TestEnvelopeRecoveryThroughHubSenderDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			for _, name := range []string{"above envelope hub", "wrong repository", "tag mismatch", "no digest"} {
				t.Run(name, func(t *testing.T) {
					request := dto.SelfUpdateEnvelope{OpID: "valid-operation", TargetVersion: "1.0.1", TargetReference: repository("agent") + ":1.0.1@sha256:" + testDigest, HubVersion: "1.0.1"}
					switch name {
					case "above envelope hub":
						request.HubVersion = "1.0.0"
					case "wrong repository":
						request.TargetReference = repository("hub") + ":1.0.1@sha256:" + testDigest
					case "tag mismatch":
						request.TargetReference = repository("agent") + ":1.0.0@sha256:" + testDigest
					case "no digest":
						request.TargetReference = repository("agent") + ":1.0.1"
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					var response dto.SelfUpdateResponse
					err = f.hub.environments.ProxyJSONRequestForEnvironment(t.Context(), f.remote, http.MethodPost, "/api/environments/0/system/upgrade", body, &response)
					require.Error(t, err)
					f.daemon.mu.Lock()
					mutations := append([]string(nil), f.daemon.mutations...)
					f.daemon.mu.Unlock()
					require.Empty(t, mutations, "agent validation caused Engine effects")
				})
			}
			// The incompatible version producer is real. Recovery reads remain usable.
			var check dto.SelfUpdateCheck
			require.NoError(t, f.hub.environments.ProxyJSONRequestForEnvironment(t.Context(), f.remote, http.MethodGet, "/api/environments/0/system/upgrade/check", nil, &check))
			require.Equal(t, "1.0.0", check.Version)
		})
	}
}
func TestBrowserEnvelopeCannotBypassHubOwnerDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			body := []byte(`{"opId":"browser-forgery","targetVersion":"1.0.1","targetReference":"` + repository("agent") + `:1.0.1@sha256:` + testDigest + `","hubVersion":"99.0.0"}`)
			router := f.hubRouter(t)
			request := httptest.NewRequest(http.MethodPost, "/api/environments/example-remote/system/upgrade", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			// If routing regresses, synchronize with the actual Engine effect rather
			// than letting a quick202 response hide the asynchronous helper start.
			if f.posts.Load() != 0 && recorder.Code >= 200 && recorder.Code < 300 {
				select {
				case <-f.daemon.created:
				case <-time.After(5 * time.Second):
					t.Fatal("forwarded update did not reach the Engine")
				}
			}
			f.daemon.mu.Lock()
			mutations := append([]string(nil), f.daemon.mutations...)
			f.daemon.mu.Unlock()
			require.Empty(t, mutations, "browser envelope caused Engine mutations")
			require.Zero(t, f.posts.Load(), "browser request was raw-proxied to the agent")
			require.Equal(t, 409, recorder.Code, recorder.Body.String())
		})
	}
}
func TestLostReplyReservesOnceThenObservesDirectAndEdge(t *testing.T) {
	updateStamp(t)
	// A 408 from an intermediary is a lost reply too, never an agent refusal.
	for _, lost := range []int{503, http.StatusRequestTimeout} {
		for _, edge := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%d", map[bool]string{false: "direct", true: "edge"}[edge], lost), func(t *testing.T) {
				f := newUpdateTransport(t, edge)
				configureUpdateFeed(t, f)
				f.loseStatus = lost
				f.loseReply.Store(true)
				result, err := f.hub.StartUpdate(t.Context(), f.remote.ID, dto.SelfUpdateEnvelope{}, "operator", "operator")
				require.NoError(t, err)
				require.NotEmpty(t, result.RunID)
				select {
				case <-f.daemon.created:
				case <-time.After(5 * time.Second):
					t.Fatal("helper create/start was not observed")
				}
				select {
				case <-f.post:
				case <-time.After(5 * time.Second):
					t.Fatal("lost reply was not observed")
				}
				var run activity.Activity
				// Synchronise with the hub's decision: it either settles the run or observes.
				require.Eventually(t, func() bool {
					require.NoError(t, f.db.First(&run, "id=?", result.RunID).Error)
					return f.postChecks.Load() > 0 || string(run.Status) != "running"
				}, 10*time.Second, 20*time.Millisecond)
				require.Equal(t, "running", string(run.Status))
				// Repeating the exact agent envelope is read-only, even after a lost reply.
				request := dto.SelfUpdateEnvelope{OpID: result.OpID, TargetVersion: "1.0.1", TargetReference: repository("agent") + ":1.0.1@sha256:" + testDigest, HubVersion: "1.0.1"}
				body, _ := json.Marshal(request)
				var repeated dto.SelfUpdateResponse
				require.NoError(t, f.hub.environments.ProxyJSONRequestForEnvironment(t.Context(), f.remote, http.MethodPost, "/api/environments/0/system/upgrade", body, &repeated))
				require.Equal(t, result.OpID, repeated.OpID)
				f.daemon.mu.Lock()
				creates := append([]map[string]any(nil), f.daemon.creates...)
				f.daemon.mu.Unlock()
				require.Len(t, creates, 1)
				require.Equal(t, request.TargetReference, creates[0]["Image"])
				// Stop automatic observers so the assertion owns the exact settlement barrier.
				f.hub.Stop()
				f.agent.Stop()
				require.NoError(t, core.WriteJSON(core.ResultPath(f.agent.data, result.OpID), core.Result{OpID: result.OpID, TargetRef: request.TargetReference, Phase: "healthy"}))
				f.version.Store("1.0.1")
				done, err := f.agent.observer().Advance(t.Context())
				require.NoError(t, err)
				require.True(t, done)
				done, err = f.hub.advanceRemote(t.Context(), f.hub.remoteStore(f.remote.ID))
				require.NoError(t, err)
				require.True(t, done)
				require.NoError(t, f.db.First(&run, "id=?", result.RunID).Error)
				require.Equal(t, "success", string(run.Status))
				var count int64
				require.NoError(t, f.db.Model(&activity.Activity{}).Where("type=?", "agent.update").Count(&count).Error)
				require.EqualValues(t, 1, count)
			})
		}
	}
}
func TestIdentityFenceRejectsHeldReply(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	store := f.hub.remoteStore(f.remote.ID)
	op := core.Operation{OpID: "held-reply", Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: f.remote.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	op, _, err := f.hub.reserveRun(t.Context(), store, op, "operator", "operator")
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&f.remote).Update("credential_revision", f.remote.CredentialRevision+1).Error)
	// Even a joined healthy reply cannot settle success after a credential change.
	require.NoError(t, f.hub.finishRemote(t.Context(), store, op, "healthy", ""))
	terminal, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, core.CodeUnknown, terminal.Code)
	require.Equal(t, "unknown", terminal.Outcome)
	// An exact-value transition cannot settle the operation again.
	changed, err := store.Change(t.Context(), op.OpID, func(*gorm.DB, *core.Operation) error { t.Fatal("settled operation callback reran"); return nil })
	require.NoError(t, err)
	require.False(t, changed)
}

func TestBackupAdmissionSharesUpdateHostGate(t *testing.T) {
	db := updateDB(t)
	store := core.NewStore(db)
	op := core.Operation{OpID: "backup-exclusion", Role: "hub", EnvironmentID: "0", TargetRef: "example.com/hub:1.0.1", TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	_, _, err := store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	engine := &backup.Engine{}
	engine.SetHostAdmission(func(ctx context.Context) error { return core.CheckHostTx(db.WithContext(ctx), "0") })
	_, _, err = engine.TryAcquireRun(t.Context(), "volume-backup", "data")
	require.ErrorContains(t, err, "host.busy")
}

func TestIdentityChangeDiscardsActualHeldReplyDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		for _, deleted := range []bool{false, true} {
			name := map[bool]string{false: "direct", true: "edge"}[edge] + map[bool]string{false: "-rotation", true: "-reregister"}[deleted]
			t.Run(name, func(t *testing.T) {
				f := newUpdateTransport(t, edge)
				f.hub.Stop()
				store := f.hub.remoteStore(f.remote.ID)
				op := core.Operation{OpID: "actual-held", Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: f.remote.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
				op, _, err := f.hub.reserveRun(t.Context(), store, op, "operator", "operator")
				require.NoError(t, err)
				_, _, err = f.agent.store.Reserve(t.Context(), op, nil)
				require.NoError(t, err)
				_, err = f.agent.store.Change(t.Context(), op.OpID, func(_ *gorm.DB, current *core.Operation) error {
					current.Phase = "settled"
					current.Outcome = "healthy"
					return nil
				})
				require.NoError(t, err)
				f.version.Store("1.0.1")
				f.holdCheck.Store(true)
				answer := make(chan error, 1)
				go func() { _, err := f.hub.advanceRemote(t.Context(), store); answer <- err }()
				select {
				case <-f.checkEntered:
				case <-time.After(5 * time.Second):
					t.Fatal("transport did not reach held reply")
				}
				if deleted {
					require.NoError(t, f.db.Delete(&f.remote).Error)
					replacement := f.remote
					replacement.CredentialRevision++
					// Reuse the external host key with a new durable registration revision.
					require.NoError(t, f.db.Unscoped().Delete(&f.remote).Error)
					require.NoError(t, f.db.Create(&replacement).Error)
				} else {
					require.NoError(t, f.db.Model(&f.remote).Update("credential_revision", f.remote.CredentialRevision+1).Error)
				}
				close(f.checkRelease)
				select {
				case err := <-answer:
					require.NoError(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("held reply did not join")
				}
				current, err := store.Read(t.Context())
				require.NoError(t, err)
				require.NotEqual(t, "healthy", current.Outcome, "superseded reply settled success")
				_, err = f.hub.advanceRemote(t.Context(), store)
				require.NoError(t, err)
				current, err = store.Read(t.Context())
				require.NoError(t, err)
				require.Equal(t, "unknown", current.Outcome)
				require.Equal(t, core.CodeUnknown, current.Code)
			})
		}
	}
}
