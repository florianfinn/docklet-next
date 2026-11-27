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
	dto "github.com/florianfinn/docklet-next/types/proxy"
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
