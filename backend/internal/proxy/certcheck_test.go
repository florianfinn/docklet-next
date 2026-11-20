// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

type probeObserveEngine struct {
	Engine
	failure     error
	observation Observation
	check       func(context.Context, dto.Instance, dto.CertificateCheckRequest) (dto.CertificateObservation, error)
}

func (e *probeObserveEngine) Observe(context.Context, Intent, bool) (Observation, error) {
	return e.observation, e.failure
}
func (e *probeObserveEngine) CheckCertificate(ctx context.Context, i dto.Instance, q dto.CertificateCheckRequest) (dto.CertificateObservation, error) {
	return e.check(ctx, i, q)
}

func TestProbeTransientObserveKeepsJournalAndRecovers(t *testing.T) {
	x := newFixture(t)
	failure := &volumehelper.DockerError{Err: errors.New("synthetic helper start failure")}
	e := &probeObserveEngine{Engine: x.e, failure: failure, observation: Observation{State: dto.Verified, Evidence: dto.Routing}}
	x.a.Engines[dto.Traefik] = e
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, failure)
	var row Journal
	require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
	require.Nil(t, row.FinishedAt, "a transient helper failure must keep the journal unfinished")
	require.Equal(t, dto.Observing, row.Phase)
	require.Equal(t, dto.Pending, row.State)
	e.failure = nil
	restarted, err := NewAgent(x.db.DB, x.f, Engines{dto.Traefik: e})
	require.NoError(t, err)
	o, err := restarted.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
	require.NotNil(t, row.FinishedAt)
}
func TestProbeTypedObserveErrorStillFinalizes(t *testing.T) {
	x := newFixture(t)
	x.a.Engines[dto.Traefik] = &probeObserveEngine{Engine: x.e, failure: dto.Fail(dto.NotObservable, 409)}
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.NotObservable, o.Reason)
	var row Journal
	require.NoError(t, x.db.First(&row, "id = ?", o.ID).Error)
	require.NotNil(t, row.FinishedAt)
}
func TestProbeObservationPersistsWithoutPromotingRouting(t *testing.T) {
	for _, state := range []dto.State{dto.Verified, dto.AppliedUnverified} {
		t.Run(string(state), func(t *testing.T) {
			x := newFixture(t)
			cert := &dto.CertificateObservation{InstanceID: x.request.Instance.ID, ObservedAt: time.Now().UTC(), ChainTrusted: false, HostnameMatch: true, Validity: dto.CertificateExpired, Peer: dto.CertificatePeer{SANs: []string{}}}
			evidence := &dto.ProbeEvidence{Client: dto.HelperClient, ImageID: "sha256:agent-local"}
			x.a.Engines[dto.Traefik] = &probeObserveEngine{Engine: x.e, observation: Observation{State: state, Certificate: cert, Probe: evidence}}
			o, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, state, o.State)
			require.Equal(t, cert, o.Certificate)
			require.Equal(t, evidence, o.Probe)
			o, err = x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, cert, o.Certificate)
			require.False(t, o.Certificate.ChainTrusted)
		})
	}
}
func TestManualCertificateCheckReadOnlyAndGated(t *testing.T) {
	x := newFixture(t)
	x.a.Gate = x.mods
	calls := 0
	x.a.Engines[dto.Traefik] = &probeObserveEngine{Engine: x.e, check: func(_ context.Context, i dto.Instance, q dto.CertificateCheckRequest) (dto.CertificateObservation, error) {
		calls++
		return dto.CertificateObservation{InstanceID: i.ID}, nil
	}}
	q := dto.HostCertificateCheckRequest{Instance: x.request.Instance, CertificateCheckRequest: dto.CertificateCheckRequest{File: "routes/app.yml", Router: "app", Host: "app.example.test", SourceHash: strings.Repeat("a", 64)}}
	_, err := x.a.CheckCertificate(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	for _, table := range []string{"proxy_journal", "proxy_operations", "proxy_reservations"} {
		var n int64
		require.NoError(t, x.db.Table(table).Count(&n).Error)
		require.Zero(t, n)
	}
	require.NoError(t, x.settings.UpdateSetting(t.Context(), "moduleProxyEnabled", "false"))
	_, err = x.a.CheckCertificate(t.Context(), q)
	require.Error(t, err)
	require.Equal(t, 1, calls)
}
func TestManualCertificateSourceFence(t *testing.T) {
	q := dto.CertificateCheckRequest{File: "routes/app.yml", Router: "app", Host: "app.example.test", SourceHash: strings.Repeat("a", 64)}
	read := dto.ReadResult{Files: []dto.File{{Path: q.File, Hash: q.SourceHash}}, Routes: []dto.Route{{File: q.File, Name: q.Router}}}
	_, err := CertificateRoute(read, q)
	require.NoError(t, err)
	read.Files[0].Hash = strings.Repeat("b", 64)
	_, err = CertificateRoute(read, q)
	requireCode(t, err, dto.RouteChanged)
	for _, file := range []string{"../app.yml", "/etc/app.yml", "routes/.secret/app.yml", "a\n.yml"} {
		q.File = file
		require.Error(t, validateCertificateCheck(q))
	}
}
