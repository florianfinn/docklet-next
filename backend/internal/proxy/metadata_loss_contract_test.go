// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original metadata-loss contract, version gate and canonical-form proofs.

package proxy

import (
	"context"
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

// exampleLossToken is built at runtime so the fixture never looks like a stored secret.
var exampleLossToken = strings.Repeat("0123456789abcdef", 4)

func withLoss(q dto.ApplyRequest, l ...dto.MetadataLoss) dto.ApplyRequest {
	q.Acknowledgements.MetadataLoss = l
	return q
}

// M44 break-map row 12: agents below contract 25 never receive the acknowledgement; the hub refuses
// before any record or reservation exists. Without an acknowledgement old agents keep every write.
func TestM44MetadataLossGateBeforeAdmission(t *testing.T) {
	for v := 18; v <= 25; v++ {
		for _, acknowledged := range []bool{true, false} {
			x := newFixture(t)
			tr := &localTransport{a: x.a, reg: x.request.Registration, contractVersion: v}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			q := x.request
			if acknowledged {
				q = withLoss(q, dto.MetadataLoss{Path: q.Target, Attributes: []string{"user.example"}, Token: exampleLossToken})
			}
			_, err := h.Apply(t.Context(), q)
			var records, reservations int64
			require.NoError(t, x.db.Model(&Record{}).Count(&records).Error)
			require.NoError(t, x.db.Model(&reservation{}).Count(&reservations).Error)
			if acknowledged && v < dto.MetadataLossContractVersion {
				require.ErrorIs(t, err, environment.ErrAgentUpdateRequired, "contract %d", v)
				require.Equal(t, []int64{0, 0, 0}, []int64{int64(tr.applyCalls), records, reservations}, "contract %d", v)
				continue
			}
			require.NoError(t, err, "contract %d acknowledged %t", v, acknowledged)
			require.Equal(t, 1, tr.applyCalls, "contract %d acknowledged %t", v, acknowledged)
		}
	}
}

// M44 break-map row 13: absent, null and empty lists are one request, on the hub, the agent and the wire.
func TestM44EmptyMetadataLossKeepsContract24Bytes(t *testing.T) {
	x := newFixture(t)
	legacy, err := json.Marshal(x.request, json.Deterministic(true))
	require.NoError(t, err)
	require.False(t, strings.Contains(string(legacy), "metadataLoss"))
	var decoded dto.ApplyRequest
	require.NoError(t, json.Unmarshal(legacy, &decoded))
	again, err := json.Marshal(decoded, json.Deterministic(true))
	require.NoError(t, err)
	require.Equal(t, string(legacy), string(again), "a stored contract 24 request re-encodes byte for byte")

	want, err := Digest(x.request)
	require.NoError(t, err)
	empty := withLoss(x.request, []dto.MetadataLoss{}...)
	got, err := Digest(empty)
	require.NoError(t, err)
	require.Equal(t, want, got)

	var sent dto.ApplyRequest
	tr := &localTransport{a: x.a, reg: x.request.Registration, contractVersion: 24, beforeApply: func() {}}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: &recordingTransport{localTransport: tr, sent: &sent}}
	_, err = h.Apply(t.Context(), empty)
	require.NoError(t, err)
	wire, err := json.Marshal(sent, json.Deterministic(true))
	require.NoError(t, err)
	require.False(t, strings.Contains(string(wire), "metadataLoss"), "a contract 24 agent never sees the field")
	var stored Record
	require.NoError(t, x.db.First(&stored, "id = ?", x.request.ID).Error)
	require.Equal(t, want, stored.Digest)
	require.False(t, strings.Contains(string(stored.Request), "metadataLoss"))
}

type recordingTransport struct {
	*localTransport
	sent *dto.ApplyRequest
}

func (r *recordingTransport) Apply(ctx context.Context, q dto.ApplyRequest) (dto.Operation, error) {
	*r.sent = q
	return r.localTransport.Apply(ctx, q)
}

// M44 break-map row 15: the acknowledgement is bounded and names only the request target.
func TestM44MetadataLossValidation(t *testing.T) {
	x := newFixture(t)
	ok := dto.MetadataLoss{Path: x.request.Target, Attributes: []string{"system.posix_acl_access"}, Links: 1, Token: exampleLossToken}
	require.NoError(t, validRequest(withLoss(x.request, ok)))
	many := make([]dto.MetadataLoss, 17)
	for n := range many {
		many[n] = ok
	}
	attrs := make([]string, 65)
	for n := range attrs {
		attrs[n] = "user.example"
	}
	bad := map[string][]dto.MetadataLoss{
		"too many":       many,
		"other path":     {{Path: "other.yml", Token: exampleLossToken}},
		"duplicate":      {ok, ok},
		"negative links": {{Path: ok.Path, Links: -1, Token: exampleLossToken}},
		"too many links": {{Path: ok.Path, Links: 1025, Token: exampleLossToken}},
		"too many names": {{Path: ok.Path, Attributes: attrs, Token: exampleLossToken}},
		"empty name":     {{Path: ok.Path, Attributes: []string{""}, Token: exampleLossToken}},
		"space in name":  {{Path: ok.Path, Attributes: []string{"user.a b"}, Token: exampleLossToken}},
		"long name":      {{Path: ok.Path, Attributes: []string{strings.Repeat("a", 256)}, Token: exampleLossToken}},
		"short token":    {{Path: ok.Path, Token: exampleLossToken[:3]}},
		"upper token":    {{Path: ok.Path, Token: strings.ToUpper(exampleLossToken)}},
	}
	for name, l := range bad {
		err := validRequest(withLoss(x.request, l...))
		var p *dto.Problem
		require.ErrorAs(t, err, &p, name)
		require.Equal(t, dto.InvalidInput, p.Code, name)
	}
}

// M44 break-map row 12 at the web entry: through the real host transport (agent contract read from the
// agent's app-version), CreateOperation refuses before admission, adoption or any record.
func TestM44CreateOperationGateBeforeAdmission(t *testing.T) {
	fingerprints := map[int]string{20: contract.AgentHubV20Fingerprint, 21: contract.AgentHubV21Fingerprint, 22: contract.AgentHubV22Fingerprint, 23: contract.AgentHubV23Fingerprint, 24: contract.AgentHubV24Fingerprint}
	for v, fp := range fingerprints {
		f := newAdoptionFixture(t)
		f.agent = &version.Info{AgentContractVersion: v, AgentContractFingerprint: fp}
		f.discover = func() { t.Errorf("contract %d: discovery before the gate", v) }
		f.inventory = func(dto.Instance) { t.Errorf("contract %d: admission before the gate", v) }
		before, err := f.m.Instance(t.Context(), f.request.Instance.ID)
		require.NoError(t, err)
		_, err = f.m.CreateOperation(t.Context(), f.request.Instance.ID, dto.OperationRequest{Target: f.request.Target, Expected: f.request.Expected, GraphHashes: f.request.GraphHashes, Change: f.request.Change, Acknowledgements: dto.Acknowledgements{MetadataLoss: []dto.MetadataLoss{{Path: f.request.Target, Token: exampleLossToken}}}})
		require.ErrorIs(t, err, environment.ErrAgentUpdateRequired, "contract %d", v)
		after, err := f.m.Instance(t.Context(), f.request.Instance.ID)
		require.NoError(t, err)
		require.Equal(t, before, after, "contract %d", v)
		require.Zero(t, f.count(t), "contract %d", v)
	}
}

// contract24Request was encoded by main 723c0de3 (contract 24); contract24Digest is main's Digest of it.
const contract24Request = `{"id":"example-contract24","instance":{"id":"instance-one","environmentId":"example-host","name":"","kind":"traefik","identity":{"containerId":"example-container","containerName":"example-proxy","startedAt":"2026-11-01T00:00:00Z"},"source":{"kind":"project","canonical":"/example/projects/proxy","mountTarget":"/etc/proxy","readOnly":false},"authority":"","routeDirectory":"","generation":"example-generation","registration":"example-registration","eligibility":{"writable":true,"textOnly":false,"line":0}},"generation":"example-generation","registration":"example-registration","target":"route.yml","expected":{"present":true,"hash":"example-hash"},"graphHashes":{"route.yml":"example-hash"},"change":{"kind":"edit-route"},"acknowledgements":{"unresolvedReferences":["example-middleware"],"replaceRunningConfig":false,"restart":false}}`
const contract24Digest = "b31d5818637763f82de5a6bc3d550c54d3435e3c6c2ce4c5d81d84c5a3a122e7"

// M44 break-map row 13 against main's bytes: hub records and agent journals written by contract 24
// keep their digest, also when a client sends null or an empty list.
func TestM44Contract24RecordsReplayUnchanged(t *testing.T) {
	variants := map[string]string{
		"absent": contract24Request,
		"null":   strings.Replace(contract24Request, `"restart":false`, `"restart":false,"metadataLoss":null`, 1),
		"empty":  strings.Replace(contract24Request, `"restart":false`, `"restart":false,"metadataLoss":[]`, 1),
	}
	for name, raw := range variants {
		require.NotEqual(t, name != "absent", raw == contract24Request, name)
		var q dto.ApplyRequest
		require.NoError(t, json.Unmarshal([]byte(raw), &q), name)
		d, err := Digest(q)
		require.NoError(t, err)
		require.Equal(t, contract24Digest, d, name)
	}
	var legacy dto.ApplyRequest
	require.NoError(t, json.Unmarshal([]byte(contract24Request), &legacy))
	again, err := json.Marshal(legacy, json.Deterministic(true))
	require.NoError(t, err)
	require.Equal(t, contract24Request, string(again))

	// Hub: a record stored by main is found again, not a conflict, and nothing is dispatched.
	x := newFixture(t)
	q := legacy
	q.Instance = x.request.Instance
	q.Generation, q.Registration = x.request.Generation, x.request.Registration
	stored, err := Digest(q)
	require.NoError(t, err)
	request, err := marshal(q)
	require.NoError(t, err)
	result, err := marshal(dto.Operation{Kind: dto.RouteOperation, ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: stored, State: dto.Verified, Phase: dto.Observing, Target: q.Target})
	require.NoError(t, err)
	require.NoError(t, x.db.Create(&Record{ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Digest: stored, Registration: q.Registration, State: dto.Verified, Request: request, Result: result}).Error)
	tr := &localTransport{a: x.a, reg: q.Registration}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	empty := q
	empty.Acknowledgements.MetadataLoss = []dto.MetadataLoss{}
	o, err := h.Apply(t.Context(), empty)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Zero(t, tr.applyCalls)

	// Agent: a journal entry from main replays as the same operation.
	x.persist(t, Intent{Request: q}, dto.Operation{Kind: dto.RouteOperation, ID: q.ID, InstanceID: q.Instance.ID, Generation: q.Generation, Registration: q.Registration, Digest: stored, State: dto.Verified, Phase: dto.Observing, Target: q.Target})
	o, err = x.a.Apply(t.Context(), empty)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
}
