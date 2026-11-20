// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 contract foundation; no proxy effects.

package proxy

import (
	"context"
	"github.com/florianfinn/docklet-next/types/features"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// These typed stubs perform no host call, admission, journal write or reservation.
func (m *Module) stubInstance(ctx context.Context, id string) error {
	i, err := m.Instance(ctx, id)
	if err != nil {
		return err
	}
	env := i.EnvironmentID
	if env != "0" {
		if m.Transport == nil || m.Transport.Hosts == nil {
			return dto.Fail(dto.AgentUnavailable, 503)
		}
		reason := m.Transport.Hosts.ModuleReason(ctx, env, features.Proxy)
		if reason == "agent.update-required" {
			return &moduledto.Problem{Status: 409, Code: reason, Feature: features.Proxy}
		}
		if reason != "" {
			return dto.Fail(dto.AgentUnavailable, 503)
		}
	}
	return dto.Fail(dto.NotImplemented, 501)
}
