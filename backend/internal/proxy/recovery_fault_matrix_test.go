// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original crash-boundary recovery matrix against durable agent and hub state.

package proxy

import (
	"errors"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP4EveryRealCrashBoundaryAndTargetSide(t *testing.T) {
	for _, mode := range []string{"traefik-edit", "traefik-create", "caddy-edit", "caddy-create", "caddy-delete", "caddy-restart"} {
		phases := []dto.Phase{dto.Admitted, dto.Staging, dto.Publishing, dto.Observing, dto.Restoring}
		if strings.HasPrefix(mode, "traefik") {
			phases = append(phases, dto.Marking)
		} else if mode == "caddy-restart" {
			phases = append(phases, dto.Restarting)
		} else {
			phases = append(phases, dto.Reloading)
		}
		for _, phase := range phases {
			for _, boundary := range []string{"commit", "effect"} {
				if phase == dto.Admitted && boundary == "effect" {
					continue
				} // Admission has no host effect.
				for _, side := range []string{"old", "new", "third"} {
					for _, entry := range []string{"startup", "query"} {
						t.Run(mode+"/"+string(phase)+"/"+boundary+"/"+side+"/"+entry, func(t *testing.T) {
							x := newFixture(t)
							if strings.HasPrefix(mode, "caddy") {
								x.request.Instance.Kind = dto.Caddy
							}
							old, newSide := Snapshot([]byte("old"), true), Snapshot([]byte("new"), true)
							if strings.HasSuffix(mode, "create") {
								old = Snapshot(nil, false)
								delete(x.f.files, x.request.Target)
								x.request.Change.Kind = "create-route"
							}
							if mode == "caddy-delete" {
								newSide = Snapshot(nil, false)
								x.request.Change.Kind = "delete-route"
							}
							if mode == "caddy-restart" {
								x.e.adminOff = true
								x.request.Acknowledgements.Restart = true
							}
							x.e.new = newSide
							x.request.Expected = old.FileState
							var graphErr error
							x.request.GraphHashes, graphErr = x.f.Graph(t.Context(), x.request.Instance)
							require.NoError(t, graphErr)
							if phase == dto.Restoring {
								x.e.observe = func() Observation { return Observation{Restore: true, Reason: dto.ProxyRejected} }
							}
							if phase == dto.Marking {
								x.e.observe = func() Observation {
									if x.e.marker == 0 {
										return Observation{NeedsMarker: true}
									}
									return Observation{State: dto.Verified, Evidence: dto.Load}
								}
							}
							reached := false
							fault := func(p dto.Phase) error {
								if p == phase {
									reached = true
									return errCrash
								}
								return nil
							}
							if boundary == "commit" {
								x.a.afterCommit = fault
							} else {
								x.a.afterEffect = fault
							}
							tr := &localTransport{a: x.a, reg: x.request.Registration, queryErr: errors.New("connection interrupted")}
							h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
							_, err := h.Apply(t.Context(), x.request)
							require.Error(t, err)
							require.True(t, reached, "must fault at the actual persisted/effect boundary")
							j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
							require.NoError(t, err)
							require.Equal(t, phase, j.Phase)
							intent, _, err := decodeJournal(j)
							require.NoError(t, err)
							actual := old
							if side == "new" {
								actual = newSide
							}
							if side == "third" {
								actual = Snapshot([]byte("operator"), true)
							}
							if actual.Present {
								x.f.files[x.request.Target] = actual
							} else {
								delete(x.f.files, x.request.Target)
							}
							// Recreate the process-owned agent; hooks and the active-operation map
							// cannot survive the crash. Keep only the durable journal and host state.
							x.a, err = NewAgent(x.db.DB, x.f, Engines{dto.Traefik: x.e, dto.Caddy: x.e})
							require.NoError(t, err)
							tr.a = x.a
							tr.queryErr = nil
							x.e.observe = nil
							reloadBefore, restartBefore := x.e.reload, x.e.restart
							want := dto.RecoveryRequired
							reload, restart := 0, 0
							switch {
							case side == "third":
							case phase == dto.Restoring:
								want = dto.Restored
								if strings.HasPrefix(mode, "caddy") {
									if mode == "caddy-restart" {
										restart = 1
									} else {
										reload = 1
									}
								}
							case side == "new" && phase == dto.Admitted:
								// Admission commits before the snapshot: new bytes are not yet known (AUTO-17).
							case side == "old":
								if phase == dto.Admitted || phase == dto.Staging || phase == dto.Publishing {
									want = dto.Rejected
								}
							default:
								want = dto.Verified
								if strings.HasPrefix(mode, "caddy") {
									if mode == "caddy-restart" {
										if phase == dto.Restarting && boundary == "commit" {
											want = dto.RecoveryRequired
										} else if phase != dto.Restarting && phase != dto.Observing {
											restart = 1
										}
									} else {
										reload = 1
									}
								}
							}
							if entry == "startup" {
								require.NoError(t, x.a.Settle(t.Context()))
							}
							got, err := h.Query(t.Context(), x.request.ID)
							require.NoError(t, err)
							require.Equal(t, want, got.State)
							require.Equal(t, reload, x.e.reload-reloadBefore)
							require.Equal(t, restart, x.e.restart-restartBefore)
							expected := actual
							if want == dto.Restored {
								expected = old
							}
							after, err := x.f.Target(t.Context(), x.request.Instance, x.request.Target)
							require.NoError(t, err)
							require.Equal(t, expected.FileState, after.FileState, "presence and hash")
							require.Equal(t, expected.Content, after.Content)
							require.NotContains(t, x.f.files, intent.TempPath)
							require.NotContains(t, x.f.files, intent.MarkerPath)
							var held, reserved int64
							require.NoError(t, x.db.Model(&Journal{}).Where("source = ? AND finished_at IS NULL", "host\x00"+x.request.Instance.Source.Canonical).Count(&held).Error)
							require.NoError(t, x.db.Model(&reservation{}).Where("operation_id = ?", x.request.ID).Count(&reserved).Error)
							if want == dto.RecoveryRequired {
								require.EqualValues(t, 1, held)
								require.EqualValues(t, 1, reserved)
							} else {
								require.Zero(t, held)
								require.Zero(t, reserved)
							}
						})
					}
				}
			}
		}
	}
}
