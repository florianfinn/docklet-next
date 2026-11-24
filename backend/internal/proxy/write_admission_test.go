// Source: docs/design/proxy-module.md
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M38 #592 write-admission guards: restart refresh for resolver writes and the row CAS retry.

package proxy

import (
	"context"
	"errors"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

// admittedResolver records that the hub admitted a resolver write and sent it.
type admittedResolver struct {
	*localTransport
	sent int
}

func (t *admittedResolver) CreateResolver(context.Context, dto.ResolverApplyRequest) (dto.Operation, error) {
	t.sent++
	return dto.Operation{}, errors.New("offline")
}

func resolverInput() dto.ResolverRequest {
	value := "example-endpoint"
	return dto.ResolverRequest{ResolverConfig: dto.ResolverConfig{Name: "Example", Email: "admin@example.test", CA: dto.ResolverCA{Kind: "letsencrypt"}, Storage: "/certs/acme.json", Challenge: dto.ResolverChallenge{Kind: "dns", Provider: dto.HTTPRequest}}, Variables: []dto.ResolverVariable{{Name: "HTTPREQ_ENDPOINT", Value: &value}}}
}

// A plain restart keeps the container and changes StartedAt; a resolver write
// stores the refreshed identity, so the hub's identity fence admits it.
func TestS2ResolverWriteAfterRestart(t *testing.T) {
	f := newAdoptionFixture(t)
	i := f.request.Instance
	i.Authority = dto.FileAuthority
	require.NoError(t, f.m.saveInstance(t.Context(), i))
	f.candidate.Identity = i.Identity
	f.candidate.Identity.StartedAt = nextEpoch
	tr := &admittedResolver{localTransport: &localTransport{a: f.a, reg: f.request.Registration, queryErr: errors.New("offline")}}
	f.m.Hub.Transport = tr
	_, err := f.m.CreateResolver(t.Context(), i.ID, resolverInput())
	var p *dto.Problem
	if errors.As(err, &p) {
		require.NotEqual(t, dto.ContainerChanged, p.Code)
	}
	require.Equal(t, 1, tr.sent)
	saved, err := f.m.Instance(t.Context(), i.ID)
	require.NoError(t, err)
	require.Equal(t, nextEpoch, saved.Identity.StartedAt)
	require.EqualValues(t, 1, f.count(t)) // the resolver record only: a restart adopts nothing
}

// A concurrent settings save between snapshot and store is retried on the new
// row and keeps it; a row that keeps changing is an operation conflict.
func TestS2WriteAdmissionRetriesRowConflict(t *testing.T) {
	for _, tt := range []struct {
		name    string
		changes int
		code    dto.Reason
	}{{"one concurrent save", 1, ""}, {"repeated saves", 2, dto.OperationConflict}} {
		t.Run(tt.name, func(t *testing.T) {
			f := newAdoptionFixture(t)
			f.candidate.Identity = f.request.Instance.Identity
			saves := 0
			f.inventory = func(dto.Instance) {
				if saves < tt.changes {
					saves++
					current, err := f.m.Instance(context.Background(), f.request.Instance.ID)
					require.NoError(t, err)
					current.Name = "concurrent-name-" + string(rune('0'+saves))
					require.NoError(t, f.m.saveInstance(context.Background(), current))
				}
			}
			instance, err := f.m.writeAdmission(t.Context(), f.request.Instance.ID, true)
			if tt.code != "" {
				requireCode(t, err, tt.code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "concurrent-name-1", instance.Name)
			saved, err := f.m.Instance(t.Context(), f.request.Instance.ID)
			require.NoError(t, err)
			require.Equal(t, "concurrent-name-1", saved.Name)
		})
	}
}
