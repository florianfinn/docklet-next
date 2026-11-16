// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original S2 R3 guard proofs: container identity, epochs, receipt identity and journal consistency.

package proxy

import (
	"errors"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func requireCode(t *testing.T, err error, code dto.Reason) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, code, p.Code)
}

// Check and Epoch on the real Docker inspect seam guard every new host effect.
func TestCheckAndEpochRealDockerSeam(t *testing.T) {
	root := t.TempDir()
	base := container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	for _, tt := range []struct {
		name         string
		change       func(*container.InspectResponse)
		check, epoch dto.Reason
	}{
		{"same", func(*container.InspectResponse) {}, "", ""},
		{"other container", func(c *container.InspectResponse) { c.ID = "example-recreated" }, dto.ContainerChanged, dto.ContainerChanged},
		{"no state", func(c *container.InspectResponse) { c.State = nil }, "", dto.ContainerChanged},
		{"changed source", func(c *container.InspectResponse) { c.Mounts[0].Source = root + "-other" }, dto.SourceReadOnly, ""},
		{"changed kind", func(c *container.InspectResponse) { c.Mounts[0].Type = mount.TypeVolume }, dto.SourceReadOnly, ""},
		// Proxy-side mount access is not writer authority (Q-SRC); the source stays the same.
		{"proxy-side read-only mount", func(c *container.InspectResponse) { c.Mounts[0].RW = false }, "", ""},
		{"other mount target", func(c *container.InspectResponse) { c.Mounts[0].Destination = "/etc/elsewhere" }, dto.SourceReadOnly, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
			c := base
			c.Mounts = append([]container.MountPoint{}, base.Mounts...)
			tt.change(&c)
			d := dockerSeam(t, x, &c)
			i := x.request.Instance
			i.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
			if err := d.Check(t.Context(), i); tt.check == "" {
				require.NoError(t, err)
			} else {
				requireCode(t, err, tt.check)
			}
			var running bool
			var startedAt string
			var err error
			require.NotPanics(t, func() { running, startedAt, err = d.Epoch(t.Context(), i) })
			if tt.epoch != "" {
				requireCode(t, err, tt.epoch)
				return
			}
			require.NoError(t, err)
			require.True(t, running)
			require.Equal(t, initialEpoch, startedAt)
		})
	}
}

// Epoch guards around observation: an unreadable, stopped or restarted container makes
// the result applied-unverified (container-changed); a requested restart may change it.
func TestApplyEpochGuards(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(x *fixture)
		want  dto.State
	}{
		{"unchanged", func(*fixture) {}, dto.Verified},
		{"epoch unreadable before observing", func(x *fixture) { x.f.epochErr = errors.New("example inspect failure") }, dto.AppliedUnverified},
		{"stopped before observing", func(x *fixture) { x.f.running = false }, dto.AppliedUnverified},
		{"restarted before observing", func(x *fixture) { x.f.epoch = nextEpoch }, dto.AppliedUnverified},
		{"restarted while observing", func(x *fixture) {
			x.e.observe = func() Observation {
				x.f.mu.Lock()
				x.f.epoch = nextEpoch
				x.f.mu.Unlock()
				return Observation{State: dto.Verified, Evidence: dto.Routing}
			}
		}, dto.AppliedUnverified},
		{"epoch unreadable after observing", func(x *fixture) {
			x.e.observe = func() Observation {
				x.f.mu.Lock()
				x.f.epochErr = errors.New("example inspect failure")
				x.f.mu.Unlock()
				return Observation{State: dto.Verified, Evidence: dto.Routing}
			}
		}, dto.AppliedUnverified},
		{"requested restart", func(x *fixture) {
			x.request.Instance.Kind, x.request.Acknowledgements.Restart, x.e.adminOff = dto.Caddy, true, true
		}, dto.Verified},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			tt.setup(x)
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, tt.want, o.State)
			if tt.want == dto.AppliedUnverified {
				// An unreadable epoch is unconfirmed identity, not an observed change (closing review C-B2).
				want := dto.ContainerChanged
				if strings.HasPrefix(tt.name, "epoch unreadable") {
					want = dto.NotObservable
				}
				require.Equal(t, want, o.Reason)
			}
		})
	}
}

// Query and Release answer only the registration and instance that own the journal.
func TestAgentRefusesOtherRegistrationOrInstance(t *testing.T) {
	for _, field := range []string{"registration", "instance"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			q := x.query()
			if field == "registration" {
				q.Registration = "example-other-registration"
			} else {
				q.Instance.ID = "example-other-instance"
			}
			_, err = x.a.Query(t.Context(), q)
			requireCode(t, err, dto.RegistrationChanged)
			_, err = x.a.Release(t.Context(), dto.ReleaseRequest{Query: q, Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
			requireCode(t, err, dto.RegistrationChanged)
		})
	}
}

// accept: a dispatch answer with another registration, ID or digest is not recorded.
func TestHubAcceptRefusesForeignAnswer(t *testing.T) {
	for _, field := range []string{"registration", "id", "digest"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			tr := &localTransport{a: x.a, reg: x.request.Registration, applyMutate: func(o *dto.Operation) {
				switch field {
				case "registration":
					o.Registration = "example-other-registration"
				case "id":
					o.ID = "example-other-operation"
				default:
					o.Digest = "example-other-digest"
				}
			}}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			_, _ = h.Apply(t.Context(), x.request)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
			require.Equal(t, dto.OperationConflict, row.Reason)
		})
	}
}

// A journal row whose columns disagree with its stored result is journal-lost.
func TestInconsistentJournalIsJournalLost(t *testing.T) {
	for _, tt := range []struct{ name, column, value string }{
		{"unknown state", "state", "example-state"},
		{"unknown phase", "phase", "example-phase"},
		{"other state", "state", string(dto.Pending)},
		{"other phase", "phase", string(dto.Staging)},
		{"other digest", "digest", "example-other-digest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.NoError(t, x.db.Exec("UPDATE proxy_journal SET "+tt.column+" = ? WHERE id = ?", tt.value, x.request.ID).Error)
			_, err = x.a.Query(t.Context(), x.query())
			requireCode(t, err, dto.JournalLost)
		})
	}
	// The same unknown value in the column and the stored result is still refused.
	for _, column := range []string{"state", "phase"} {
		t.Run("unknown "+column+" in both", func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.NoError(t, x.db.Exec("UPDATE proxy_journal SET "+column+" = ?, result = json_set(result, '$."+column+"', ?) WHERE id = ?", "example-value", "example-value", x.request.ID).Error)
			_, err = x.a.Query(t.Context(), x.query())
			requireCode(t, err, dto.JournalLost)
		})
	}
}

// A hub record whose state column disagrees with its stored result is journal-lost.
func TestInconsistentHubRecordIsRefused(t *testing.T) {
	for _, value := range []string{"example-state", string(dto.RecoveryRequired)} {
		t.Run(value, func(t *testing.T) {
			x := newFixture(t)
			persistRecord(t, x, dto.Pending)
			require.NoError(t, x.db.Exec("UPDATE proxy_operations SET state = ? WHERE id = ?", value, x.request.ID).Error)
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &localTransport{a: x.a, reg: x.request.Registration}}
			_, err := h.Query(t.Context(), x.request.ID)
			requireCode(t, err, dto.JournalLost)
		})
	}
	t.Run("unknown state in both", func(t *testing.T) {
		x := newFixture(t)
		persistRecord(t, x, dto.Pending)
		require.NoError(t, x.db.Exec("UPDATE proxy_operations SET state = ?, result = json_set(result, '$.state', ?) WHERE id = ?", "example-value", "example-value", x.request.ID).Error)
		h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &localTransport{a: x.a, reg: x.request.Registration}}
		_, err := h.Query(t.Context(), x.request.ID)
		requireCode(t, err, dto.JournalLost)
	})
}

// A lost dispatch whose fence answers generation-mismatch or journal-lost becomes
// recovery-required with that reason.
func TestHubFenceRefusalBecomesRecovery(t *testing.T) {
	for _, code := range []dto.Reason{dto.GenerationMismatch, dto.JournalLost} {
		t.Run(string(code), func(t *testing.T) {
			x := newFixture(t)
			tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true, noAdmission: true, fenceErr: dto.Fail(code, 409)}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			_, _ = h.Apply(t.Context(), x.request)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
			require.Equal(t, code, row.Reason)
		})
	}
	// Any other fence failure, typed or not, leaves the record unfinished without a recovery reason.
	t.Run("plain error", func(t *testing.T) {
		x := newFixture(t)
		tr := &localTransport{a: x.a, reg: x.request.Registration, drop: true, noAdmission: true, fenceErr: errors.New("example transport failure")}
		h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
		require.NotPanics(t, func() { _, _ = h.Apply(t.Context(), x.request) })
		var row Record
		require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
		require.NotEqual(t, dto.RecoveryRequired, row.State)
	})
}
