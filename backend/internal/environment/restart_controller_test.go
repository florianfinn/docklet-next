// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original restart durable-state, fencing and retained-outcome tests.

package environment

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type restartFixture struct {
	c            *restartController
	url          string
	now          time.Time
	instance     string
	calls        int
	sendStatus   int
	sendError    bool
	unreachable  bool
	incompatible bool
	duringHealth func()
}

func newRestartFixture(t *testing.T) *restartFixture {
	t.Helper()
	f := &restartFixture{url: sqlitestore.URL(filepath.Join(t.TempDir(), "restart.db")), now: time.Now().UTC().Truncate(time.Microsecond), instance: "example-old", sendStatus: 202}
	db, err := database.Initialize(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	f.c = &restartController{ctx: t.Context(), now: func() time.Time { return f.now }, wait: restartWait}
	f.c.service = NewEnvironmentService(db, &http.Client{Transport: agentRoundTrip(f.roundTrip)}, nil, nil, nil, nil)
	require.NoError(t, db.Create(&Environment{ID: "example-host", Name: "Example host", ApiUrl: "https://agent.example.com", Enabled: true, AccessToken: new("example")}).Error)
	t.Cleanup(func() { require.NoError(t, f.c.service.db.Close()) })
	return f
}
func (f *restartFixture) roundTrip(r *http.Request) (*http.Response, error) {
	status := 200
	body := []byte(`{}`)
	switch r.URL.Path {
	case "/api/health":
		if f.unreachable {
			return nil, errors.New("synthetic unreachable")
		}
		instance := f.instance
		if f.duringHealth != nil {
			fn := f.duringHealth
			f.duringHealth = nil
			fn()
		}
		body, _ = json.Marshal(map[string]string{"status": "UP", "instanceId": instance})
	case "/api/app-version":
		info := version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
		if f.incompatible {
			info.AgentContractVersion--
		}
		body, _ = json.Marshal(info)
	case "/api/environments/0/system/health":
		if r.Header.Get("X-Arcane-Agent-Token") != "example" {
			status = 401
		}
	case "/api/system/agent/restart":
		f.calls++
		status = f.sendStatus
		if f.sendError {
			return nil, errors.New("synthetic lost acknowledgement")
		}
		if status == 409 {
			body = []byte(`{"code":"agent.restart-unsupported","status":409}`)
		} else {
			body = []byte(`{"instanceId":"response-is-only-evidence"}`)
		}
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}, Request: r}, nil
}
func (f *restartFixture) reserve(t *testing.T) *restartWork {
	t.Helper()
	row, err := f.c.reserve(t.Context(), "example-host")
	require.NoError(t, err)
	return &restartWork{row: row}
}
func (f *restartFixture) step(t *testing.T, w *restartWork) bool {
	t.Helper()
	done, err := f.c.advance(t.Context(), w)
	require.NoError(t, err)
	return done
}
func (f *restartFixture) reopen(t *testing.T, w *restartWork) {
	t.Helper()
	require.NoError(t, f.c.service.db.Close())
	db, err := database.Initialize(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	f.c.service = NewEnvironmentService(db, &http.Client{Transport: agentRoundTrip(f.roundTrip)}, nil, nil, nil, nil)
	require.NoError(t, db.First(&w.row, "run_id=?", w.row.RunID).Error)
	w.recovered = true
	w.pending = nil
}
func assertRestartCode(t *testing.T, w *restartWork, state, code string) {
	t.Helper()
	require.Equal(t, state, w.row.State)
	if code == "" {
		require.Nil(t, w.row.Code)
	} else {
		require.NotNil(t, w.row.Code)
		require.Equal(t, code, *w.row.Code)
	}
}

func TestRestartSameInstanceCompatibilityAndOneSend(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	require.False(t, f.step(t, w))
	require.Equal(t, 1, f.calls)
	require.Equal(t, "awaiting", w.row.State)
	require.NotNil(t, w.row.AcknowledgedAt)
	require.Equal(t, "example-old", w.row.OldInstance)
	for range 3 {
		require.False(t, f.step(t, w))
		require.Nil(t, w.row.SettledAt)
	}
	f.instance = "example-new"
	f.incompatible = true
	require.False(t, f.step(t, w))
	require.Nil(t, w.row.SettledAt)
	f.incompatible = false
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "connected", "")
	require.Equal(t, 1, f.calls)
	// Terminal truth survives late responses, credential edits, deadlines and process reopen.
	require.NoError(t, f.c.service.db.Model(&Environment{}).Where("id=?", "example-host").Update("credential_revision", 1).Error)
	f.instance = "example-old"
	f.now = f.now.Add(5 * time.Minute)
	f.reopen(t, w)
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "connected", "")
	require.Equal(t, 1, f.calls)
}
func TestRestartRecoveryAllPersistedStatesAndDeadlines(t *testing.T) {
	for _, state := range []string{"reserved", "sent", "awaiting", "connected", "failed", "unknown"} {
		for _, ack := range []bool{false, true} {
			if state == "reserved" && ack {
				continue
			}
			t.Run(state+"/"+map[bool]string{true: "ack", false: "no-ack"}[ack], func(t *testing.T) {
				f := newRestartFixture(t)
				w := f.reserve(t)
				if state != "reserved" {
					now := f.now
					w.row.State = state
					w.row.SentAt = &now
					w.row.OldInstance = f.instance
					if ack {
						w.row.AcknowledgedAt = &now
					}
				}
				if state == "connected" || state == "failed" || state == "unknown" {
					w.row.SettledAt = &f.now
					if state == "failed" {
						w.row.Code = new("agent.restart-timeout")
					}
					if state == "unknown" {
						w.row.Code = new("agent.restart-unconfirmed")
					}
				}
				require.NoError(t, f.c.service.db.Save(&w.row).Error)
				f.reopen(t, w)
				if state == "reserved" {
					require.True(t, f.step(t, w))
					assertRestartCode(t, w, "failed", "agent.restart-interrupted")
				} else if w.row.SettledAt != nil {
					before := w.row
					require.True(t, f.step(t, w))
					require.Equal(t, before, w.row)
				} else {
					require.False(t, f.step(t, w))
					require.Nil(t, w.row.SettledAt)
					f.now = w.row.Deadline
					require.True(t, f.step(t, w))
					if ack {
						assertRestartCode(t, w, "failed", "agent.restart-timeout")
					} else {
						assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
					}
				}
				require.Zero(t, f.calls, "recovery must never replay a POST")
			})
		}
	}
}
func TestRestartLostAcknowledgementUnsupportedAndMissingEvidence(t *testing.T) {
	for _, kind := range []string{"lost", "unsupported", "no-health"} {
		t.Run(kind, func(t *testing.T) {
			f := newRestartFixture(t)
			w := f.reserve(t)
			f.sendError = kind == "lost"
			if kind == "unsupported" {
				f.sendStatus = 409
			}
			f.unreachable = kind == "no-health"
			done := f.step(t, w)
			switch kind {
			case "lost":
				require.False(t, done)
				require.Nil(t, w.row.AcknowledgedAt)
				f.reopen(t, w)
				f.now = w.row.Deadline
				require.True(t, f.step(t, w))
				assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
				require.Equal(t, 1, f.calls)
			case "unsupported":
				require.True(t, done)
				assertRestartCode(t, w, "failed", "agent.restart-unsupported")
				require.Equal(t, 1, f.calls)
			case "no-health":
				require.True(t, done)
				assertRestartCode(t, w, "failed", "host.unreachable")
				require.Zero(t, f.calls)
			}
		})
	}
}
func TestRestartEveryActiveStateCredentialAndRegistrationFence(t *testing.T) {
	for _, state := range []string{"reserved", "sent", "awaiting"} {
		for _, change := range []string{"endpoint", "key", "manual-token", "delete-reregister", "late-old-response"} {
			t.Run(state+"/"+change, func(t *testing.T) {
				f := newRestartFixture(t)
				w := f.reserve(t)
				if state != "reserved" {
					require.False(t, f.step(t, w))
					if state == "sent" {
						w.row.State = "sent"
						require.NoError(t, f.c.service.db.Model(&w.row).Update("state", "sent").Error)
					}
				}
				mutate := func() {
					if change == "delete-reregister" {
						require.NoError(t, f.c.service.db.Delete(&Environment{}, "id=?", "example-host").Error)
						require.NoError(t, f.c.service.db.Create(&Environment{ID: "example-replacement", Name: "Example replacement", ApiUrl: "https://agent.example.com", Enabled: true, AccessToken: new("example")}).Error)

					} else if change == "endpoint" || change == "manual-token" {
						field, value := "api_url", "https://replacement.example.com"
						if change == "manual-token" {
							field, value = "access_token", "example-replacement"
						}
						updates := map[string]any{field: value}
						if field == "api_url" {
							updates["access_token"] = "example"
						}
						_, err := f.c.service.UpdateEnvironment(t.Context(), "example-host", updates, nil, nil)
						require.NoError(t, err)
					} else {
						require.NoError(t, f.c.service.db.Model(&Environment{}).Where("id=?", "example-host").Update("credential_revision", gorm.Expr("credential_revision+1")).Error)

					}
					f.c.service.invalidateEnvironmentCacheInternal("example-host")
				}
				if change == "late-old-response" {
					f.duringHealth = mutate
				} else {
					mutate()
				}
				require.True(t, f.step(t, w))
				assertRestartCode(t, w, "failed", "agent.restart-superseded")
				if state == "reserved" {
					require.Zero(t, f.calls)
				} else {
					require.Equal(t, 1, f.calls)
				}
			})
		}
	}
}
func TestRestartSettlementTransactionFencesRetainedObservation(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	require.False(t, f.step(t, w))
	f.instance = "example-new"
	// Inject at the write boundary, after compatibility/health have joined.
	fail := true
	require.NoError(t, f.c.service.db.Callback().Update().Before("gorm:update").Register("restart-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_restart_requests" && fail {
			tx.AddError(errors.New("synthetic write failure"))
		}
	}))
	done, err := f.c.advance(t.Context(), w)
	require.Error(t, err)
	require.False(t, done)
	require.NotNil(t, w.pending)
	require.Equal(t, "connected", w.pending.state)
	require.NoError(t, f.c.service.db.Model(&Environment{}).Where("id=?", "example-host").Update("credential_revision", 1).Error)
	fail = false
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "failed", "agent.restart-superseded")
	require.NoError(t, f.c.service.db.Callback().Update().Remove("restart-fail"))
}
func TestRestartWriteFailuresRetainOutcomeAndExclusion(t *testing.T) {
	for _, phase := range []string{"pre-send", "ack", "observation", "deadline", "reserved-recovery"} {
		t.Run(phase, func(t *testing.T) {
			f := newRestartFixture(t)
			w := f.reserve(t)
			if phase == "observation" || phase == "deadline" {
				require.False(t, f.step(t, w))
				f.instance = "example-new"
			}
			if phase == "deadline" {
				f.now = w.row.Deadline
			}
			if phase == "reserved-recovery" {
				f.reopen(t, w)
			}
			fail := true
			require.NoError(t, f.c.service.db.Callback().Update().Before("gorm:update").Register("restart-fail", func(tx *gorm.DB) {
				if tx.Statement.Table == "agent_restart_requests" && fail && (phase != "ack" || f.calls > 0) {
					tx.AddError(errors.New("synthetic write failure"))
				}
			}))
			done, err := f.c.advance(t.Context(), w)
			require.Error(t, err)
			require.False(t, done)
			var n int64
			require.NoError(t, f.c.service.db.Model(&AgentRestartRequest{}).Where("settled_at IS NULL").Count(&n).Error)
			require.EqualValues(t, 1, n)
			if phase == "pre-send" {
				require.Zero(t, f.calls)
			} else if phase != "reserved-recovery" {
				require.Equal(t, 1, f.calls)
			}
			// Make the original proof impossible to refetch. Only the retained joined outcome can settle.
			if phase == "pre-send" {
				f.unreachable = true
			}
			if phase == "observation" {
				f.unreachable = true
				f.now = w.row.Deadline.Add(time.Minute)
			}
			fail = false
			done = f.step(t, w)
			if phase == "pre-send" || phase == "ack" {
				require.False(t, done)
				require.Equal(t, 1, f.calls)
				require.NotNil(t, w.row.AcknowledgedAt)
			} else {
				require.True(t, done)
			}
			if phase == "observation" {
				assertRestartCode(t, w, "connected", "")
			}
			if phase == "deadline" {
				assertRestartCode(t, w, "failed", "agent.restart-timeout")
			}
			if phase == "reserved-recovery" {
				assertRestartCode(t, w, "failed", "agent.restart-interrupted")
			}
			require.NoError(t, f.c.service.db.Callback().Update().Remove("restart-fail"))
		})
	}
}
func TestRestartReservationBusyAndLocalUnsupported(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	_, err := f.c.reserve(t.Context(), "example-host")
	var p *dto.HostProblem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "host.busy", p.Code)
	_, err = f.c.reserve(t.Context(), "0")
	require.ErrorAs(t, err, &p)
	require.Equal(t, "agent.restart-unsupported", p.Code)
	require.False(t, f.step(t, w))
	require.Equal(t, 1, f.calls)
}

func TestRestartAmbiguousSentWriteNeverSendsAgain(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	// Model a committed sent transaction whose acknowledgement was lost locally.
	now := f.now
	require.NoError(t, f.c.service.db.Model(&AgentRestartRequest{}).Where("run_id=?", w.row.RunID).Updates(map[string]any{"state": "sent", "old_instance": f.instance, "sent_at": now}).Error)
	require.False(t, f.step(t, w))
	require.Equal(t, "sent", w.row.State)
	require.Zero(t, f.calls)
	f.now = w.row.Deadline
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
}
func TestRestartTerminalRowsImmutableAgainstEveryLateBoundary(t *testing.T) {
	for _, state := range []string{"connected", "failed", "unknown"} {
		t.Run(state, func(t *testing.T) {
			f := newRestartFixture(t)
			w := f.reserve(t)
			require.False(t, f.step(t, w))
			terminal := &restartOutcome{state: state, code: "agent.restart-timeout", observedAt: f.now}
			if state == "connected" {
				terminal.code = ""
			}
			if state == "unknown" {
				terminal.code = "agent.restart-unconfirmed"
			}
			require.NoError(t, f.c.persist(t.Context(), w, terminal))
			saved := w.row
			require.NoError(t, f.c.service.db.Delete(&Environment{}, "id=?", "example-host").Error)
			f.now = w.row.Deadline.Add(time.Hour)
			f.reopen(t, w)
			for _, late := range []*restartOutcome{{state: "connected", observedAt: f.now}, {state: "awaiting", acknowledged: &f.now}, {state: "failed", code: "agent.restart-superseded"}, {state: "unknown", code: "agent.restart-unconfirmed"}} {
				require.NoError(t, f.c.persist(t.Context(), w, late))
				require.Equal(t, saved, w.row)
			}
			require.True(t, f.step(t, w))
			require.Equal(t, 1, f.calls)
		})
	}
}

func TestRestartHubIntakeVersionGateZeroPost(t *testing.T) {
	f := newRestartFixture(t)
	f.incompatible = true
	f.c.service.restarts = f.c
	h := NewHandler(f.c.service, nil, nil, nil, nil, nil)
	_, err := h.RestartAgent(t.Context(), &restartAgentInput{ID: "example-host"})
	require.Error(t, err)
	require.Zero(t, f.calls)
	var n int64
	require.NoError(t, f.c.service.db.Model(&AgentRestartRequest{}).Count(&n).Error)
	require.Zero(t, n)
}
func TestRestartObservationsRecheckRegistrationAfterTransientReadFailure(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	require.False(t, f.step(t, w))
	f.instance = "example-new"
	failed := true
	require.NoError(t, f.c.service.db.Callback().Query().Before("gorm:query").Register("restart-read-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "environments" && failed {
			tx.AddError(errors.New("synthetic read failure"))
		}
	}))
	done, err := f.c.advance(t.Context(), w)
	require.Error(t, err)
	require.False(t, done)
	require.Nil(t, w.row.SettledAt)
	failed = false
	require.NoError(t, f.c.service.db.Callback().Query().Remove("restart-read-fail"))
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "connected", "")
}

func TestRestartActualStartupRecoveryReopensStoreWithoutReplay(t *testing.T) {
	for _, ack := range []bool{false, true} {
		t.Run(map[bool]string{true: "ack", false: "lost-202"}[ack], func(t *testing.T) {
			f := newRestartFixture(t)
			f.sendError = !ack
			w := f.reserve(t)
			require.False(t, f.step(t, w))
			require.Equal(t, 1, f.calls)
			f.reopen(t, w)
			// A second controller owns a new service/store lifetime. No intake is replayed.
			f.instance = "example-new"
			changed, unsubscribe := f.c.service.SubscribeRuntimeChanges()
			defer unsubscribe()
			require.NoError(t, f.c.service.StartAgentRestarts(t.Context()))
			defer f.c.service.StopAgentRestarts()
			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()
			for {
				var row AgentRestartRequest
				require.NoError(t, f.c.service.db.First(&row, "run_id=?", w.row.RunID).Error)
				if row.SettledAt != nil {
					require.Equal(t, "connected", row.State)
					require.Equal(t, w.row.Deadline, row.Deadline)
					require.Equal(t, ack, row.AcknowledgedAt != nil)
					break
				}
				select {
				case <-changed:
				case <-deadline.C:
					t.Fatal("recovered controller did not settle")
				}
			}
			require.Equal(t, 1, f.calls)
		})
	}
}

func TestRestartRecoveryScanSettlesUnclaimedReservedRow(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	changed, unsubscribe := f.c.service.SubscribeRuntimeChanges()
	defer unsubscribe()
	require.NoError(t, f.c.service.StartAgentRestarts(t.Context()))
	defer f.c.service.StopAgentRestarts()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		var row AgentRestartRequest
		require.NoError(t, f.c.service.db.First(&row, "run_id=?", w.row.RunID).Error)
		if row.SettledAt != nil {
			assertRestartCode(t, &restartWork{row: row}, "failed", "agent.restart-interrupted")
			break
		}
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("unclaimed reservation did not recover")
		}
	}
	require.Zero(t, f.calls)
}
