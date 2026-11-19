// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original stored sign-in policy with AUTH_MODE as forced override (docs/design/first-run-setup.md).

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

// ErrSetupPending refuses user credentials and identities until first-run setup created the administrator.
var ErrSetupPending = errors.New(authtypes.CodeSetupPending)

const defaultProxyHeader = "Remote-User"

// signInState is embedded in AuthService: the published snapshot (nil before InitializeSignIn means
// the env view), the memoised env view, the writer mutex and the process-local setup hooks.
type signInState struct {
	policy    atomic.Pointer[Policy]
	envPolicy atomic.Pointer[envPolicyEntry]
	policyMu  sync.Mutex
	setup     setupGuard
}

// Policy is one immutable snapshot of the effective sign-in policy; a request reads it once.
type Policy struct {
	Mode     authtypes.SignInMode
	Header   string
	Trusted  string
	Networks []netip.Prefix
	Forced   bool
	Pending  bool
}

// LocalActive reports whether local passwords, sessions, Bearer tokens and personal API keys are accepted.
func (p *Policy) LocalActive() bool {
	return p != nil && !p.Pending && p.Mode == authtypes.SignInModeLocal
}

func (p *Policy) stored() session.StoredSignInPolicy {
	return session.StoredSignInPolicy{Mode: p.Mode, ProxyHeader: p.Header, TrustedProxies: p.Trusted}
}

// policyFromConfigInternal is the env view: the forced policy, or local when AUTH_MODE is unset.
// It also serves services that never ran InitializeSignIn (agents, unit tests).
func policyFromConfigInternal(cfg *config.Config) *Policy {
	p := &Policy{Mode: authtypes.SignInModeLocal, Header: defaultProxyHeader}
	if cfg == nil {
		return p
	}
	p.Mode = cfg.SignInMode()
	_, p.Forced = cfg.ForcedSignInMode()
	p.Header = cfg.ProxyIdentityHeader()
	p.Trusted = cfg.AuthTrustedProxies
	p.Networks, _ = cfg.AuthProxyNetworks()
	return p
}

// newPolicyInternal validates a stored or requested triple.
func newPolicyInternal(stored session.StoredSignInPolicy) (*Policy, error) {
	p := &Policy{Mode: stored.Mode, Header: stored.ProxyHeader, Trusted: stored.TrustedProxies}
	if p.Mode == "" {
		p.Mode = authtypes.SignInModeLocal
	}
	if p.Header == "" {
		p.Header = defaultProxyHeader
	}
	switch p.Mode {
	case authtypes.SignInModeNone, authtypes.SignInModeLocal:
		return p, nil
	case authtypes.SignInModeProxy:
		if err := config.ValidateProxyHeader(p.Header); err != nil {
			return nil, err
		}
		networks, err := config.ParseTrustedProxies(p.Trusted)
		if err != nil {
			return nil, err
		}
		p.Networks = networks
		return p, nil
	default:
		return nil, errors.New("sign-in mode must be exactly one of none, local, proxy")
	}
}

// CurrentPolicy returns the effective snapshot. A forced policy is the env view (AUTH_MODE and its proxy
// variables), combined with the published setup state; otherwise the published stored policy applies.
// Agents always use local for their own listeners.
func (s *AuthService) CurrentPolicy() *Policy {
	if s == nil {
		return &Policy{Mode: authtypes.SignInModeLocal, Header: defaultProxyHeader}
	}
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return &Policy{Mode: authtypes.SignInModeLocal, Header: defaultProxyHeader}
	}
	published := s.policy.Load()
	if _, forced := s.config.ForcedSignInMode(); !forced && published != nil {
		return published
	}
	return s.envPolicyInternal(published != nil && published.Pending)
}

type envPolicyEntry struct {
	key     string
	pending bool
	policy  *Policy
}

// envPolicyInternal memoises the parsed env view so request paths do not reparse the proxy list.
func (s *AuthService) envPolicyInternal(pending bool) *Policy {
	key := ""
	if s.config != nil {
		key = s.config.AuthMode + "\x00" + s.config.AuthProxyHeader + "\x00" + s.config.AuthTrustedProxies
	}
	if entry := s.envPolicy.Load(); entry != nil && entry.key == key && entry.pending == pending {
		return entry.policy
	}
	p := policyFromConfigInternal(s.config)
	p.Pending = pending
	s.envPolicy.Store(&envPolicyEntry{key: key, pending: pending, policy: p})
	return p
}

func (s *AuthService) loadPolicyInternal(ctx context.Context) (*Policy, error) {
	stored, claimed, err := s.sessionService.LoadSignInState(ctx)
	if err != nil {
		return nil, err
	}
	if _, forced := s.config.ForcedSignInMode(); forced {
		p := policyFromConfigInternal(s.config)
		p.Pending = !claimed
		return p, nil
	}
	p, err := newPolicyInternal(stored)
	if err != nil {
		return nil, fmt.Errorf("stored sign-in policy is invalid (%w); set AUTH_RESET_ADMIN_PASSWORD with an empty AUTH_MODE and restart to recover", err)
	}
	p.Pending = !claimed
	return p, nil
}

// publishPolicyInternal runs write inside one transaction and publishes next only after commit.
// policyMu orders concurrent writers from transaction begin through publication.
func (s *AuthService) publishPolicyInternal(ctx context.Context, next *Policy, write func(context.Context, *Policy) error) error {
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	if err := write(ctx, next); err != nil {
		return err
	}
	s.policy.Store(next)
	s.InvalidateAllTokenCache()
	// Sessionless streams (none and proxy mode) carry no session to revoke: close every user stream so
	// it reconnects under the new policy. Machine ingress registers no user binding and stays open.
	sessionlife.Revoke("", "", "")
	return nil
}
