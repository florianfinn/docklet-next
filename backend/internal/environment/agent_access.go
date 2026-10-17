// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
)

var ErrAgentUpdateRequired = errors.New("agent update required")

type agentSession struct {
	token    string
	endpoint string
	ctx      context.Context
	cancel   context.CancelFunc
}

type agentReport struct {
	token  string
	report environmenttypes.AgentReport
}

type agentRuntime struct {
	mu       sync.Mutex
	sessions map[string]*agentSession
	reports  map[string]agentReport
}

// AgentAccess revalidates credentials and compatibility before any agent operation.
func (s *EnvironmentService) AgentAccess(ctx context.Context, id string, token *string, method, path string) (context.Context, context.CancelFunc, error) {
	env, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !env.Enabled || env.AccessToken == nil || token == nil || *env.AccessToken != *token {
		return nil, nil, ErrInvalidEnvironmentAccessToken
	}
	keyID := ""
	if env.ApiKeyID != nil {
		keyID = *env.ApiKeyID
	}
	s.agentRuntime.mu.Lock()
	if s.agentRuntime.sessions == nil {
		s.agentRuntime.sessions = make(map[string]*agentSession)
	}
	session := s.agentRuntime.sessions[id]
	if session == nil || session.token != *token || session.endpoint != env.ApiUrl {
		if session != nil {
			session.cancel()
		}
		sessionCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		session = &agentSession{token: *token, endpoint: env.ApiUrl, ctx: sessionCtx, cancel: cancel}
		s.agentRuntime.sessions[id] = session
	}
	s.agentRuntime.mu.Unlock()
	requestCtx, cancel := context.WithCancel(edge.WithSessionKey(ctx, keyID))
	stop := context.AfterFunc(session.ctx, cancel)
	cleanup := func() { stop(); cancel() }
	if !contract.AgentRecoveryRoute(method, path) {
		if _, err = s.CheckAgentCompatibility(requestCtx, id); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if err = requestCtx.Err(); err != nil {
		cleanup()
		return nil, nil, err
	}
	return requestCtx, cleanup, nil
}

// CheckAgentCompatibility uses the same transport and key binding as ordinary requests.
func (s *EnvironmentService) CheckAgentCompatibility(ctx context.Context, id string) (*version.Info, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	env, err := s.GetEnvironmentByID(checkCtx, id)
	if err != nil {
		return nil, err
	}
	var info version.Info
	if err := s.ProxyJSONRequest(checkCtx, id, http.MethodGet, "/api/app-version", nil, &info); err != nil {
		return nil, err
	}
	compatible := contract.AgentCompatible(info.AgentContractVersion, info.AgentContractFingerprint)
	status := string(EnvironmentStatusOnline)
	if !compatible {
		status = string(EnvironmentStatusUpdateRequired)
	}
	if err := checkCtx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	result := s.db.WithContext(checkCtx).Model(&Environment{}).Where("id = ?", id).Where(map[string]any{"access_token": env.AccessToken, "api_url": env.ApiUrl}).Updates(map[string]any{"status": status, "last_seen": &now, "updated_at": &now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrInvalidEnvironmentAccessToken
	}
	s.invalidateEnvironmentCacheInternal(id)
	s.agentRuntime.mu.Lock()
	if s.agentRuntime.reports == nil {
		s.agentRuntime.reports = make(map[string]agentReport)
	}
	s.agentRuntime.reports[id] = agentReport{token: *env.AccessToken, report: environmenttypes.AgentReport{Version: info, ReportedAt: now}}
	s.agentRuntime.mu.Unlock()
	s.NotifyRuntimeStateChanged()
	if !compatible {
		return &info, ErrAgentUpdateRequired
	}
	return &info, nil
}

// ResolveEdgeSessionKey rechecks the token at registration, including cached-token misses.
func (s *EnvironmentService) ResolveEdgeSessionKey(ctx context.Context, id, token string) (string, error) {
	env, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !env.Enabled || !env.IsEdge || env.AccessToken == nil || *env.AccessToken != token || env.ApiKeyID == nil {
		return "", ErrInvalidEnvironmentAccessToken
	}
	return *env.ApiKeyID, nil
}

func (s *EnvironmentService) revokeAgentSessions(id string) {
	s.agentRuntime.mu.Lock()
	if session := s.agentRuntime.sessions[id]; session != nil {
		session.cancel()
		delete(s.agentRuntime.sessions, id)
	}
	delete(s.agentRuntime.reports, id)
	s.agentRuntime.mu.Unlock()
	if tunnel, ok := edge.GetRegistry().Get(id).Get(); ok {
		_ = tunnel.CloseWithReason("agent credential revoked")
		edge.GetRegistry().Unregister(id)
	}
	edge.ForgetPollState(id)
	s.ForgetSyncState(id)
}
