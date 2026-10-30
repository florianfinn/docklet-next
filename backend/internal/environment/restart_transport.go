// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original fenced Direct and Edge restart observations and single sender.

package environment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment/children/proxy"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/system"
)

func (s *EnvironmentService) restartInstance(parent context.Context, env Environment) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if env.AccessToken == nil || !env.Enabled {
		return "", errors.New("restart host is unavailable")
	}
	if env.IsEdge {
		tunnel, ok := edge.GetRegistry().Get(env.ID).Get()
		if !ok || !edge.TunnelLiveAt(tunnel, time.Now()) {
			return "", remenv.ErrEnvironmentUnavailable
		}
		key := ""
		if env.ApiKeyID != nil {
			key = *env.ApiKeyID
		}
		if tunnel.APIKeyID != key || tunnel.TokenHash != edge.AgentTokenHash(*env.AccessToken) {
			return "", ErrInvalidEnvironmentAccessToken
		}
		before := tunnel.MetadataSnapshot()
		if before.AgentInstance == "" {
			return "", errors.New("agent instance unavailable")
		}
		if _, err := s.CheckAgentCompatibility(edge.WithSessionTunnel(ctx, tunnel), env.ID); err != nil {
			return "", err
		}
		current, ok := edge.GetRegistry().Get(env.ID).Get()
		if !ok || current != tunnel || !edge.TunnelLiveAt(current, time.Now()) {
			return "", remenv.ErrEnvironmentUnavailable
		}
		after := current.MetadataSnapshot()
		if after.SessionID != before.SessionID || after.AgentInstance != before.AgentInstance {
			return "", errors.New("agent session changed during observation")
		}
		return before.AgentInstance, nil
	}
	// Public health is bracketed by an authenticated compatibility probe and a
	// second instance read. A bare public 200 can never complete a restart.
	first, err := s.restartHealth(ctx, env)
	if err != nil {
		return "", err
	}
	if _, err = s.CheckAgentCompatibility(ctx, env.ID); err != nil {
		return "", err
	}
	second, err := s.restartHealth(ctx, env)
	if err != nil {
		return "", err
	}
	if first == "" || first != second {
		return "", errors.New("agent instance changed during observation")
	}
	return first, nil
}
func (s *EnvironmentService) restartHealth(ctx context.Context, env Environment) (string, error) {
	target, err := proxy.NewTarget(env.ID, env.Name, env.ApiUrl, env.IsEdge, env.AccessToken)
	if err != nil {
		return "", err
	}
	response, err := s.proxy.Execute(ctx, target, http.MethodGet, "/api/health", nil)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", errors.New("agent health unavailable")
	}
	var health system.HealthResponse
	if err := json.Unmarshal(response.Body, &health); err != nil {
		return "", err
	}
	if health.Status != "UP" {
		return "", errors.New("agent health is not up")
	}
	return health.InstanceID, nil
}
func (s *EnvironmentService) sendAgentRestart(parent context.Context, env Environment) (*remenv.Response, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	target, err := proxy.NewTarget(env.ID, env.Name, env.ApiUrl, env.IsEdge, env.AccessToken)
	if err != nil {
		return nil, err
	}
	return s.proxy.Execute(ctx, target, http.MethodPost, "/api/system/agent/restart", nil)
}
func restartUnsupported(response *remenv.Response) bool {
	var p dto.HostProblem
	return response.StatusCode == 409 && len(response.Body) <= 16<<10 && json.Unmarshal(response.Body, &p) == nil && p.Code == "agent.restart-unsupported"
}
