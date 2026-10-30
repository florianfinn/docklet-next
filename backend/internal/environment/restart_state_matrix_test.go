// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original persisted restart state by external boundary matrix.

package environment

import (
	"errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestRestartPersistedStateBoundaryMatrix(t *testing.T) {
	for _, state := range []string{"reserved", "sent", "awaiting", "connected", "failed", "unknown"} {
		for _, boundary := range []string{"observation", "hub-restart", "credential-change", "registration-change", "deadline", "late-response", "write-failure"} {
			t.Run(state+"/"+boundary, func(t *testing.T) {
				f := newRestartFixture(t)
				w := f.reserve(t)
				now := f.now
				w.row.State = state
				w.row.OldInstance = f.instance
				if state != "reserved" {
					w.row.SentAt = &now
				}
				if state == "awaiting" {
					w.row.AcknowledgedAt = &now
				}
				terminal := state == "connected" || state == "failed" || state == "unknown"
				if terminal {
					w.row.SettledAt = &now
					if state == "failed" {
						w.row.Code = new("agent.restart-timeout")
					}
					if state == "unknown" {
						w.row.Code = new("agent.restart-unconfirmed")
					}
				}
				require.NoError(t, f.c.service.db.Save(&w.row).Error)
				expectedState, expectedCode := state, ""
				if w.row.Code != nil {
					expectedCode = *w.row.Code
				}
				f.instance = "example-new"
				switch boundary {
				case "hub-restart":
					f.reopen(t, w)
				case "credential-change":
					require.NoError(t, f.c.service.db.Model(&Environment{}).Where("id=?", "example-host").Update("credential_revision", gorm.Expr("credential_revision+1")).Error)
				case "registration-change":
					require.NoError(t, f.c.service.db.Delete(&Environment{}, "id=?", "example-host").Error)
					require.NoError(t, f.c.service.db.Create(&Environment{ID: "example-replacement", Name: "Example replacement", ApiUrl: "https://agent.example.com", Enabled: true, AccessToken: new("example")}).Error)
				case "deadline":
					f.now = w.row.Deadline
				case "late-response":
					f.instance = "example-old"
				}
				if boundary == "write-failure" {
					fail := true
					writes := 0
					require.NoError(t, f.c.service.db.Callback().Update().Before("gorm:update").Register("restart-matrix-fail", func(tx *gorm.DB) {
						if tx.Statement.Table == "agent_restart_requests" {
							writes++
							if fail {
								tx.AddError(errors.New("synthetic matrix write failure"))
							}
						}
					}))
					done, err := f.c.advance(t.Context(), w)
					if terminal {
						require.NoError(t, err)
						require.True(t, done)
						require.Zero(t, writes)
					} else {
						require.Error(t, err)
						require.False(t, done)
						var persisted AgentRestartRequest
						require.NoError(t, f.c.service.db.First(&persisted, "run_id=?", w.row.RunID).Error)
						require.Equal(t, state, persisted.State)
						require.Nil(t, persisted.SettledAt)
						f.unreachable = true
						fail = false
						f.step(t, w)
					}
					require.NoError(t, f.c.service.db.Callback().Update().Remove("restart-matrix-fail"))
				} else {
					f.step(t, w)
				}
				if !terminal {
					switch boundary {
					case "credential-change", "registration-change":
						expectedState, expectedCode = "failed", "agent.restart-superseded"
					case "deadline":
						switch state {
						case "reserved":
							expectedState, expectedCode = "failed", "host.unreachable"
						case "sent":
							expectedState, expectedCode = "unknown", "agent.restart-unconfirmed"
						case "awaiting":
							expectedState, expectedCode = "failed", "agent.restart-timeout"
						}
					case "hub-restart":
						if state == "reserved" {
							expectedState, expectedCode = "failed", "agent.restart-interrupted"
						} else {
							expectedState = "connected"
						}
					case "late-response":
						if state == "reserved" {
							expectedState = "awaiting"
						}
					default:
						if state == "reserved" {
							expectedState = "awaiting"
						} else {
							expectedState = "connected"
						}
					}
				}
				require.NoError(t, f.c.service.db.First(&w.row, "run_id=?", w.row.RunID).Error)
				assertRestartCode(t, w, expectedState, expectedCode)
				expectedCalls := 0
				if state == "reserved" && (boundary == "observation" || boundary == "late-response" || boundary == "write-failure") {
					expectedCalls = 1
				}
				require.Equal(t, expectedCalls, f.calls)
			})
		}
	}
}
