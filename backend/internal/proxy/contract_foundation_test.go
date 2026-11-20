// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 persistence discrimination and zero-effect stub proofs.

package proxy

import (
	"encoding/json/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestM28LegacyV19RouteRestart(t *testing.T) {
	x := newFixture(t)
	digest, err := Digest(x.request)
	require.NoError(t, err)
	o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: digest, State: dto.Pending, Phase: dto.Staging, Target: x.request.Target, Old: x.request.Expected, New: x.e.new.FileState, CreatedAt: time.Now().UTC()}
	x.persist(t, Intent{Request: x.request, Old: x.f.files[x.request.Target], New: x.e.new}, o)
	var j Journal
	require.NoError(t, x.db.First(&j, "id = ?", o.ID).Error)
	require.NotContains(t, string(j.Intent), `"kind":"route"`)
	x.a, err = NewAgent(x.db.DB, x.f, x.a.Engines)
	require.NoError(t, err)
	result, err := x.a.Query(t.Context(), x.query())
	require.NoError(t, err)
	require.Equal(t, dto.Rejected, result.State)
	require.Zero(t, x.f.publications)
}
func TestM28UnknownKindNeverRunsRouteRecovery(t *testing.T) {
	for _, kind := range []string{"resolver", "future-operation", "malformed", "malformed-result"} {
		t.Run(kind, func(t *testing.T) {
			x := newFixture(t)
			persistRecord(t, x, dto.Pending)
			o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: rowDigest(t, x), State: dto.Pending, Phase: dto.Publishing, Target: x.request.Target, Old: x.request.Expected, New: x.e.new.FileState, CreatedAt: time.Now().UTC()}
			x.persist(t, Intent{Request: x.request, Old: x.f.files[x.request.Target], New: x.e.new}, o)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", o.ID).Error)
			var j Journal
			require.NoError(t, x.db.First(&j, "id = ?", o.ID).Error)
			if kind == "malformed" {
				require.NoError(t, x.db.Model(&Journal{}).Where("id = ?", o.ID).Update("intent", []byte("{")).Error)
				require.NoError(t, x.db.Model(&Record{}).Where("id = ?", o.ID).Update("request", []byte("{")).Error)
				instance, err := marshal(x.request.Instance)
				require.NoError(t, err)
				require.NoError(t, x.db.Create(&instanceRow{ID: x.request.Instance.ID, EnvironmentID: x.request.Instance.EnvironmentID, Data: instance}).Error)
			} else if kind == "malformed-result" {
				require.NoError(t, x.db.Model(&Journal{}).Where("id = ?", o.ID).Update("result", []byte("{")).Error)
				require.NoError(t, x.db.Model(&Record{}).Where("id = ?", o.ID).Update("result", []byte("{")).Error)
			} else {
				require.NoError(t, x.db.Exec("UPDATE proxy_journal SET intent = json_set(intent, '$.kind', ?) WHERE id = ?", kind, o.ID).Error)
				require.NoError(t, x.db.Exec("UPDATE proxy_operations SET request = json_set(request, '$.kind', ?) WHERE id = ?", kind, o.ID).Error)
			}
			var err error
			x.a, err = NewAgent(x.db.DB, x.f, x.a.Engines)
			require.NoError(t, err)
			require.NoError(t, x.a.Settle(t.Context()))
			result, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, result.State)
			require.Equal(t, dto.UnknownOperationKind, result.Reason)
			require.Nil(t, result.FinishedAt)
			fenced, err := x.a.Fence(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, fenced.Operation.State)
			q := x.request
			q.ID = "another-operation"
			_, err = x.a.Apply(t.Context(), q)
			requireCode(t, err, dto.ModuleBusy)
			tr := &localTransport{a: x.a, reg: x.request.Registration}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			got, err := h.Query(t.Context(), o.ID)
			require.NoError(t, err)
			require.Equal(t, dto.UnknownOperationKind, got.Reason)
			require.Zero(t, tr.queries.Load())
			var count int64
			require.NoError(t, x.db.Model(&reservation{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			released, err := h.Release(t.Context(), o.ID, x.request.Expected, "example-actor")
			require.NoError(t, err)
			require.Equal(t, dto.Released, released.State)
			again, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.Released, again.State)
			repeat, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: x.query(), Confirmed: x.request.Expected, Actor: "example-actor"})
			require.NoError(t, err)
			require.Equal(t, again, repeat)
			require.NoError(t, x.db.Model(&reservation{}).Count(&count).Error)
			require.Zero(t, count)
			require.Zero(t, x.f.publications)
			require.Zero(t, x.f.restorations)
			require.Zero(t, x.e.observations)
			require.Equal(t, "old", string(x.f.files[x.request.Target].Content))
		})
	}
}
func TestM28StubsAndEdgeCommands(t *testing.T) {
	x := newFixture(t)
	value := "example-sentinel-value"
	q := dto.ResolverApplyRequest{ID: "resolver-one", Instance: x.request.Instance, Request: dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "test", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}}, Variables: []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &value}}}}
	_, err := x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.InvalidInput)
	q.Request.Variables = append(q.Request.Variables, dto.ResolverVariable{Name: "COMPOSE_FILE"})
	_, err = x.a.CreateResolver(t.Context(), q)
	requireCode(t, err, dto.InvalidInput)
	_, err = x.a.CheckCertificate(t.Context(), dto.HostCertificateCheckRequest{Instance: x.request.Instance, CertificateCheckRequest: dto.CertificateCheckRequest{File: "routes/app.yml", Router: "app", SourceHash: strings.Repeat("a", 64), Host: "app.example.test"}})
	requireCode(t, err, dto.CertificateUnavailable)
	for command, path := range map[string]string{"proxy.certificate.check": "/api/environments/0/modules/proxy/certificate-check", "proxy.resolver.create": "/api/environments/0/modules/proxy/resolvers"} {
		require.True(t, edge.ValidateEdgeCommand(command, "POST", path, false), command)
		require.Contains(t, edge.AdvertisedEdgeCommands(), command)
	}
	for _, table := range []string{"proxy_journal", "proxy_operations", "proxy_reservations"} {
		var n int64
		require.NoError(t, x.db.Table(table).Count(&n).Error)
		require.Zero(t, n)
	}
	b, err := json.Marshal(q.Request.Redact())
	require.NoError(t, err)
	require.NotContains(t, string(b), value)
	require.Zero(t, x.f.publications)
	require.Zero(t, x.f.restorations)
}

func rowDigest(t *testing.T, x *fixture) string {
	t.Helper()
	d, err := Digest(x.request)
	require.NoError(t, err)
	return d
}

func TestM28RouteDigestPreservesV19(t *testing.T) {
	x := newFixture(t)
	legacy, err := Digest(x.request)
	require.NoError(t, err)
	q := x.request
	q.Kind = dto.RouteOperation
	explicit, err := Digest(q)
	require.NoError(t, err)
	require.Equal(t, legacy, explicit)
}

func TestM28TypedResolverEnvelopeAcrossRestart(t *testing.T) {
	for _, phase := range []dto.ResolverPhase{dto.ResolverAdmitted, dto.ResolverBackup, dto.ResolverWriting, dto.ResolverRecreating, dto.ResolverObserving, dto.ResolverApplied, dto.ResolverRolledBack, dto.ResolverRefused, dto.ResolverRestoring, dto.ResolverRecoveryRequired} {
		t.Run(string(phase), func(t *testing.T) {
			x := newFixture(t)
			intent := dto.ResolverIntent{ResolverConfig: dto.ResolverConfig{Name: "example", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "http", EntryPoint: "web"}}, Variables: []dto.ResolverVariableIntent{}}
			digest, err := dto.ResolverEnvelope{Kind: dto.ResolverOperation, ID: x.request.ID, Instance: x.request.Instance, Generation: x.a.Generation(), Registration: x.request.Registration, Intent: intent}.Digest()
			require.NoError(t, err)
			state := dto.Pending
			if phase == dto.ResolverRecoveryRequired {
				state = dto.RecoveryRequired
			}
			if phase.Final() {
				state = dto.State(phase)
			}
			o := dto.Operation{Kind: dto.ResolverOperation, ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: digest, State: state, Phase: dto.Phase(phase), CreatedAt: time.Now().UTC(), Resolver: &dto.ResolverReceipt{Intent: intent, Phase: phase, Files: []dto.ResolverFile{}}}
			if state.Final() {
				now := time.Now().UTC()
				o.FinishedAt = &now
			}
			envelope := dto.ResolverEnvelope{Kind: dto.ResolverOperation, ID: o.ID, Instance: x.request.Instance, Generation: o.Generation, Registration: o.Registration, Intent: intent}
			raw, err := marshal(envelope)
			require.NoError(t, err)
			result, err := marshal(o)
			require.NoError(t, err)
			require.NoError(t, x.db.Create(&Journal{ID: o.ID, Source: "host\x00" + x.request.Instance.Source.Canonical, Generation: o.Generation, Registration: o.Registration, Digest: digest, State: state, Phase: o.Phase, Intent: raw, Result: result, CreatedAt: o.CreatedAt, FinishedAt: o.FinishedAt}).Error)
			require.NoError(t, x.db.Create(&Record{ID: o.ID, InstanceID: o.InstanceID, Generation: o.Generation, Registration: o.Registration, Digest: digest, State: state, Request: raw, Result: result, CreatedAt: o.CreatedAt, FinishedAt: o.FinishedAt}).Error)
			journal, err := findJournal(t.Context(), x.db.DB, o.ID)
			require.NoError(t, err)
			_, got, err := decodeJournal(journal)
			require.NoError(t, err)
			require.Equal(t, phase, got.Resolver.Phase)
			require.Equal(t, state, got.State)
			var record Record
			require.NoError(t, x.db.First(&record, "id = ?", o.ID).Error)
			_, hubResult, err := decodeRecord(record)
			require.NoError(t, err)
			require.Equal(t, got.Resolver, hubResult.Resolver)
			require.Zero(t, x.f.publications)
			require.Zero(t, x.f.restorations)
			require.Zero(t, x.e.observations)
		})
	}
}
