// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original protocol regression and deterministic recovery proofs.

package proxy

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestP3FinalGraphFence(t *testing.T) {
	for _, change := range []string{"target", "add", "remove", "mount"} {
		t.Run(change, func(t *testing.T) {
			x := newFixture(t)
			x.f.files["sibling.yml"] = Snapshot([]byte("sibling"), true)
			x.request.GraphHashes, _ = x.f.Graph(t.Context(), x.request.Instance)
			x.e.validate = func() error {
				x.f.mu.Lock()
				defer x.f.mu.Unlock()
				switch change {
				case "target":
					x.f.files["route.yml"] = Snapshot([]byte("operator"), true)
				case "add":
					x.f.files["new.yml"] = Snapshot([]byte("added"), true)
				case "remove":
					delete(x.f.files, "sibling.yml")
				case "mount":
					x.f.checkErr = dto.Fail(dto.SourceReadOnly, 409)
				}
				return nil
			}
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.NotEqual(t, dto.Verified, o.State)
			require.Zero(t, x.f.publications, "final graph check must precede publication")
			if change == "target" {
				require.Equal(t, "operator", string(x.f.files["route.yml"].Content))
			}
		})
	}
}
func TestP12DuplicateDigestAndFence(t *testing.T) {
	x := newFixture(t)
	first, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, first.State)
	for range 20 {
		again, err := x.a.Apply(t.Context(), cloneRequest(x.request))
		require.NoError(t, err)
		require.Equal(t, first, again, "receipt lookup must precede stale-hash checks")
	}
	require.Equal(t, 1, x.f.publications)
	for _, mutate := range []func(*dto.ApplyRequest){func(q *dto.ApplyRequest) { q.Target = "other.yml" }, func(q *dto.ApplyRequest) { q.Acknowledgements.Restart = true }, func(q *dto.ApplyRequest) { q.Acknowledgements.UnresolvedReferences = []string{"example@file"} }} {
		q := cloneRequest(x.request)
		mutate(&q)
		_, err = x.a.Apply(t.Context(), q)
		require.ErrorContains(t, err, "operation-conflict")
	}
	q := x.query()
	q.ID = "abandoned"
	r, err := x.a.Fence(t.Context(), q)
	require.NoError(t, err)
	require.True(t, r.Fenced)
	next := x.request
	next.ID = q.ID
	_, err = x.a.Apply(t.Context(), next)
	require.ErrorContains(t, err, "operation-fenced")
}
func TestP13SourceLockAcrossInstancesAndFiles(t *testing.T) {
	x := newFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	x.e.observe = func() Observation {
		if x.e.observations == 1 {
			close(entered)
			<-release
		}
		return Observation{State: dto.Verified, Evidence: dto.Routing}
	}
	go func() { _, err := x.a.Apply(t.Context(), x.request); done <- err }()
	<-entered
	second := x.request
	second.ID = "apply-two"
	second.Instance.ID = "instance-two"
	second.Target = "other.yml"
	second.Expected = dto.FileState{}
	_, err := x.a.Apply(t.Context(), second)
	require.ErrorContains(t, err, "module-busy")
	require.Equal(t, 1, x.f.publications)
	close(release)
	require.NoError(t, <-done)
	x.e.observe = nil
	second.GraphHashes, _ = x.f.Graph(t.Context(), second.Instance)
	_, err = x.a.Apply(t.Context(), second)
	require.NoError(t, err)
}
func TestP8ObservationEpochFence(t *testing.T) {
	for _, boundary := range []string{"restart", "recreate"} {
		t.Run(boundary, func(t *testing.T) {
			x := newFixture(t)
			x.e.observe = func() Observation {
				x.f.mu.Lock()
				x.f.epoch = nextEpoch
				if boundary == "recreate" {
					x.f.checkErr = dto.Fail(dto.ContainerChanged, 409)
				}
				x.f.mu.Unlock()
				return Observation{State: dto.Verified, Evidence: dto.Routing}
			}
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, dto.AppliedUnverified, o.State)
			require.Equal(t, dto.ContainerChanged, o.Reason)
			require.Zero(t, x.f.restorations)
		})
	}
}
func TestP4RecoveryTable(t *testing.T) {
	for _, mode := range []string{"traefik-edit", "traefik-create", "caddy-edit", "caddy-create", "caddy-delete", "caddy-restart"} {
		for _, phase := range []dto.Phase{dto.Admitted, dto.Staging, dto.Publishing, dto.Reloading, dto.Restarting, dto.Observing, dto.Marking, dto.Restoring} {
			for _, side := range []string{"old", "new", "third"} {
				for _, entry := range []string{"query", "startup"} {
					t.Run(mode+"/"+string(phase)+"/"+side+"/"+entry, func(t *testing.T) {
						invalidPhase := (phase == dto.Restarting && mode != "caddy-restart") || (phase == dto.Reloading && (mode[:5] != "caddy" || mode == "caddy-restart")) || (phase == dto.Marking && mode[:5] == "caddy")

						x := newFixture(t)
						if mode[:5] == "caddy" {
							x.request.Instance.Kind = dto.Caddy
						}
						if mode == "caddy-restart" {
							x.e.adminOff = true
							x.request.Acknowledgements.Restart = true
						}
						old, newSide := Snapshot([]byte("old"), true), Snapshot([]byte("new"), true)
						if mode == "traefik-create" || mode == "caddy-create" {
							old = Snapshot(nil, false)
						}
						if mode == "caddy-delete" {
							newSide = Snapshot(nil, false)
						}
						i := Intent{Request: x.request, Old: old, New: newSide, Mode: 0644, GraphHashes: x.request.GraphHashes, Reload: x.request.Instance.Kind == dto.Caddy && !x.e.adminOff, Restart: x.e.adminOff, StartedBefore: initialEpoch}
						paths(&i)
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
						x.f.files[i.TempPath] = Snapshot([]byte("temporary"), true)
						x.f.files[i.MarkerPath] = Snapshot([]byte("marker"), true)
						x.f.epoch = initialEpoch
						if i.Restart {
							x.f.epoch = nextEpoch
						}
						digest, err := Digest(x.request)
						require.NoError(t, err)
						o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: digest, State: dto.Pending, Phase: phase, Target: x.request.Target, Old: old.FileState, New: newSide.FileState, CreatedAt: time.Now().UTC()}
						x.persist(t, i, o)
						requestBytes, er := marshal(x.request)
						require.NoError(t, er)
						resultBytes, er := marshal(o)
						require.NoError(t, er)
						require.NoError(t, x.db.Create(&Record{ID: o.ID, InstanceID: o.InstanceID, Generation: o.Generation, Digest: o.Digest, Registration: o.Registration, State: o.State, Request: requestBytes, Result: resultBytes, CreatedAt: o.CreatedAt}).Error)
						require.NoError(t, x.db.Create(&reservation{InstanceID: o.InstanceID, OperationID: o.ID}).Error)
						h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &localTransport{a: x.a, reg: x.request.Registration}}
						if entry == "startup" {
							require.NoError(t, x.a.Settle(t.Context()))
						}
						got, err := h.Query(t.Context(), x.request.ID)
						require.NoError(t, err)
						want := dto.RecoveryRequired
						reload, restart := 0, 0
						switch {
						case side == "third" || invalidPhase:
						case phase == dto.Restoring:
							want = dto.Restored
							if i.Reload {
								reload = 1
							}
							if i.Restart {
								restart = 1
							}
						case side == "old":
							if phase == dto.Admitted || phase == dto.Staging || phase == dto.Publishing {
								want = dto.Rejected
							}
						default:
							want = dto.Verified
							if i.Reload && (phase == dto.Admitted || phase == dto.Staging || phase == dto.Publishing || phase == dto.Reloading || phase == dto.Observing) {
								reload = 1
							}
							if i.Restart && (phase == dto.Admitted || phase == dto.Staging || phase == dto.Publishing || phase == dto.Reloading) {
								restart = 1
							}
						}
						require.Equal(t, want, got.State)
						var reserved int64
						require.NoError(t, x.db.Model(&reservation{}).Where("operation_id = ?", o.ID).Count(&reserved).Error)
						if want == dto.RecoveryRequired {
							require.EqualValues(t, 1, reserved)
						} else {
							require.Zero(t, reserved)
						}
						require.Equal(t, reload, x.e.reload)
						require.Equal(t, restart, x.e.restart)
						after, _ := x.f.Target(t.Context(), i.Request.Instance, i.Request.Target)
						expected := actual
						if want == dto.Restored {
							expected = old
						}
						require.Equal(t, expected.FileState, after.FileState, "recovery must preserve third states and restore only old/new")
						_, hasTemp := x.f.files[i.TempPath]
						_, hasMarker := x.f.files[i.MarkerPath]
						require.False(t, hasTemp)
						require.False(t, hasMarker)
						var held int64
						require.NoError(t, x.db.Model(&Journal{}).Where("source = ? AND finished_at IS NULL", "host\x00"+x.request.Instance.Source.Canonical).Count(&held).Error)
						if want == dto.RecoveryRequired {
							require.EqualValues(t, 1, held)
						} else {
							require.Zero(t, held)
						}
					})
				}
			}
		}
	}
}
func TestP4RestartStoppedAndRestoreFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "failure", false: "restore"}[fail], func(t *testing.T) {
			x := newFixture(t)
			x.request.Instance.Kind = dto.Caddy
			x.request.Acknowledgements.Restart = true
			x.e.adminOff = true
			x.a.afterCommit = func(p dto.Phase) error {
				if p == dto.Restarting {
					return errCrash
				}
				return nil
			}
			_, err := x.a.Apply(t.Context(), x.request)
			require.ErrorIs(t, err, errCrash)
			x.a.afterCommit = nil
			x.f.running = false
			if fail {
				x.e.restartErr = errors.New("start failed")
			}
			o, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			if fail {
				require.Equal(t, dto.RecoveryRequired, o.State)
			} else {
				require.Equal(t, dto.Restored, o.State)
			}
			require.Equal(t, "old", string(x.f.files["route.yml"].Content))
			require.Equal(t, 1, x.e.restart)
		})
	}
}
func TestP4FaultWindows(t *testing.T) {
	for _, kind := range []dto.Kind{dto.Traefik, dto.Caddy} {
		for _, where := range []string{"commit", "effect"} {
			for _, phase := range []dto.Phase{dto.Admitted, dto.Staging, dto.Publishing, dto.Reloading, dto.Observing, dto.Marking, dto.Restoring, dto.Restarting} {
				t.Run(string(kind)+"/"+where+"/"+string(phase), func(t *testing.T) {
					if (where == "effect" && phase == dto.Admitted) || (kind == dto.Traefik && (phase == dto.Reloading || phase == dto.Restarting)) || (kind == dto.Caddy && phase == dto.Marking) {
						t.Skip("phase has no effect in this engine mode")
					}

					x := newFixture(t)
					x.request.Instance.Kind = kind
					if phase == dto.Restarting {
						x.e.adminOff = true
						x.request.Acknowledgements.Restart = true
					}
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
					fault := func(p dto.Phase) error {
						if p == phase {
							return errCrash
						}
						return nil
					}
					if where == "commit" {
						x.a.afterCommit = fault
					} else {
						x.a.afterEffect = fault
					}
					_, err := x.a.Apply(t.Context(), x.request)
					require.Error(t, err, "the configured crash boundary must be reached")
					require.ErrorIs(t, err, errCrash)
					x.a.afterCommit = nil
					x.a.afterEffect = nil
					x.e.reloadErr = nil
					x.e.observe = nil
					require.NoError(t, x.a.Settle(t.Context()))
					o, err := x.a.Query(t.Context(), x.query())
					require.NoError(t, err)
					require.True(t, o.State.Valid())
				})
			}
		}
	}
}
func TestP24AgentRelease(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Publishing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	x.f.files["route.yml"] = Snapshot([]byte("operator"), true)
	o, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	q := dto.ReleaseRequest{Query: x.query(), Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"}
	got, err := x.a.Release(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, dto.Released, got.State)
	require.Equal(t, "example-admin", got.ReleasedBy)
	require.Equal(t, "operator", string(x.f.files["route.yml"].Content))
	id, err := unfinished(t.Context(), x.db.DB, "proxy_journal")
	require.NoError(t, err)
	require.Empty(t, id)
}

func TestP4UnknownExecExitRetainsRecovery(t *testing.T) {
	x := newFixture(t)
	x.request.Instance.Kind = dto.Caddy
	x.e.reloadErr = context.DeadlineExceeded
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	require.Equal(t, dto.ExecRunning, o.Reason)
	require.Equal(t, 1, x.e.reload)
	require.Zero(t, x.f.restorations)
	_, err = x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, 1, x.e.reload, "query must not launch a contradicting reload after a timed-out exec")
}
func TestFinalSettlementSurvivesRequestCancellation(t *testing.T) {
	x := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	x.e.observe = func() Observation {
		cancel()
		return Observation{State: dto.AppliedUnverified, Reason: dto.NotObservable}
	}
	o, err := x.a.Apply(ctx, x.request)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
	require.NoError(t, err)
	require.NotNil(t, j.FinishedAt)
}

func TestOwnedSnapshotsKeepOnlyVerifiedBytes(t *testing.T) {
	x := newFixture(t)
	_, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	var n int64
	require.NoError(t, x.db.Model(&verifiedFile{}).Count(&n).Error)
	require.Zero(t, n, "an engine cannot claim ownership of a foreign file")
	q := x.request
	q.ID = "create-owned"
	q.Target = "owned.yml"
	q.Expected = dto.FileState{}
	q.GraphHashes, _ = x.f.Graph(t.Context(), q.Instance)
	_, err = x.a.Apply(t.Context(), q)
	require.NoError(t, err)
	var row verifiedFile
	require.NoError(t, x.db.First(&row, "path = ?", q.Target).Error)
	require.Equal(t, x.e.new.Hash, row.Hash)
	require.Equal(t, x.e.new.Content, row.Content)
	q.ID = "create-unverified"
	q.Target = "unverified.yml"
	q.GraphHashes, _ = x.f.Graph(t.Context(), q.Instance)
	x.e.observe = func() Observation { return Observation{State: dto.AppliedUnverified, Reason: dto.NotObservable} }
	_, err = x.a.Apply(t.Context(), q)
	require.NoError(t, err)
	row = verifiedFile{}
	require.NoError(t, x.db.First(&row, "path = ?", q.Target).Error)
	require.Empty(t, row.Hash, "unverified bytes must never be offered as last good bytes")
}

func TestCaddyNeverEntersMarkerPhase(t *testing.T) {
	x := newFixture(t)
	x.request.Instance.Kind = dto.Caddy
	x.e.observe = func() Observation { return Observation{NeedsMarker: true} }
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.NoObservableDifference, o.Reason)
	require.Zero(t, x.e.marker)
}
