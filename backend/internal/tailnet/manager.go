// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original generation-fenced lifecycle of the hub's embedded tailnet node.

package tailnet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"

	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// Credential is the decrypted hub credential; it never leaves the hub process.
type Credential struct {
	Mode         string `json:"mode"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AuthKey      string `json:"authKey,omitempty"`
}

// secrets lists the values redacted from every log line besides the tskey- pattern.
func (c Credential) secrets() []string { return []string{c.ClientSecret, c.AuthKey} }

func (c Credential) configured() bool {
	switch c.Mode {
	case tailnettypes.ModeOAuth:
		return strings.TrimSpace(c.ClientSecret) != ""
	case tailnettypes.ModeManual:
		return strings.TrimSpace(c.AuthKey) != ""
	}
	return false
}

// ManagerConfig holds process configuration; zero durations take the documented defaults.
type ManagerConfig struct {
	StateDir          string
	Hostname          string
	ControlURL        string
	MTLSDisabled      bool
	Factory           NodeFactory
	Provisioner       *Provisioner
	Now               func() time.Time
	JoinTimeout       time.Duration // 60 s per join attempt
	RetryMin          time.Duration // 5 s
	RetryMax          time.Duration // 60 s
	RefreshInterval   time.Duration // at most one Status refresh per 5 s
	ControlGrace      time.Duration // Self offline longer than 60 s means control unreachable
	ProvisionRetryMin time.Duration // 30 s between credential checks after a provider failure
	ProvisionRetryMax time.Duration // 5 min
}

func (c *ManagerConfig) defaults() {
	if c.Factory == nil {
		c.Factory = NewTSNetNode
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if strings.TrimSpace(c.Hostname) == "" {
		c.Hostname = "docklet-hub"
	}
	for _, d := range []struct {
		v   *time.Duration
		def time.Duration
	}{{&c.JoinTimeout, 60 * time.Second}, {&c.RetryMin, 5 * time.Second}, {&c.RetryMax, 60 * time.Second}, {&c.RefreshInterval, 5 * time.Second}, {&c.ControlGrace, 60 * time.Second}, {&c.ProvisionRetryMin, 30 * time.Second}, {&c.ProvisionRetryMax, 5 * time.Minute}} {
		if *d.v <= 0 {
			*d.v = d.def
		}
	}
}

// Manager owns the hub's single tailnet node. Every Apply or Reset starts a new
// generation; results of older generations are dropped.
type Manager struct {
	cfg ManagerConfig

	testMu       sync.Mutex
	bootID       string
	credCounter  uint64
	mu           sync.Mutex
	gen          int64
	enabled      bool
	cred         Credential
	run          *run
	handler      http.Handler
	conn         tailnettypes.Connection
	prov         tailnettypes.Provisioning
	status       *ipnstate.Status
	coordWarning bool // bus health reports the control server not delivering network maps
	offlineSince time.Time
	provSeq      int64 // last started provisioning operation
	provApplied  int64 // newest provisioning operation whose result was recorded
	listeners    map[int]func()
	nextListener int
}

type run struct {
	gen      int64
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	node     Node
	listener net.Listener
	ingress  *ingressServer
}

// NewManager returns a stopped manager.
func NewManager(cfg ManagerConfig) *Manager {
	cfg.defaults()
	return &Manager{
		cfg:       cfg,
		bootID:    newBootID(),
		conn:      tailnettypes.Connection{State: "off"},
		prov:      tailnettypes.Provisioning{State: "not-configured"},
		listeners: map[int]func(){},
	}
}

// Subscribe registers a change callback; callbacks run without the manager lock.
func (m *Manager) Subscribe(fn func()) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextListener
	m.nextListener++
	m.listeners[id] = fn
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.listeners, id)
	}
}

func (m *Manager) notifyLocked() []func() {
	out := make([]func(), 0, len(m.listeners))
	for _, fn := range m.listeners {
		out = append(out, fn)
	}
	return out
}

func fire(fns []func()) {
	for _, fn := range fns {
		fn()
	}
}

// SetIngressHandler installs the shared handler behind the allowlist; a running node starts serving it.
func (m *Manager) SetIngressHandler(handler http.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
	if r := m.run; r != nil && r.listener != nil && r.ingress == nil && handler != nil {
		r.ingress = startIngress(r.listener, handler)
	}
}

// Apply replaces the desired state. The old node, listener and watcher are closed before a new run starts.
func (m *Manager) Apply(ctx context.Context, enabled bool, cred Credential) {
	if m.updateCredential(enabled, cred) {
		return
	}
	m.mu.Lock()
	m.credCounter++
	gen, old, fns := m.restartLocked(enabled, cred)
	m.mu.Unlock()
	m.startGeneration(ctx, gen, old, fns, enabled, cred)
}

// restartLocked reserves the generation before releasing the lifecycle lock.
func (m *Manager) restartLocked(enabled bool, cred Credential) (int64, *run, []func()) {
	m.gen++
	gen := m.gen
	old := m.run
	m.run = nil
	m.enabled = enabled
	m.cred = cred
	m.status = nil
	m.coordWarning = false
	m.offlineSince = time.Time{}
	m.provApplied = m.provSeq
	m.prov = initialProvisioning(enabled, cred)
	if enabled && cred.configured() {
		m.conn = tailnettypes.Connection{State: "joining"}
	} else {
		m.conn = tailnettypes.Connection{State: "off"}
	}
	fns := m.notifyLocked()

	return gen, old, fns
}

func (m *Manager) startGeneration(ctx context.Context, gen int64, old *run, fns []func(), enabled bool, cred Credential) {
	stopRun(ctx, old)
	fire(fns)
	if !enabled || !cred.configured() {
		return
	}
	if !m.cfg.MTLSDisabled {
		m.setConnection(gen, tailnettypes.Connection{State: "error", Code: tailnettypes.CodeMTLSUnsupported})
		return
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r := &run{gen: gen, ctx: runCtx, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		cancel()
		return
	}
	m.run = r
	m.mu.Unlock()
	go func() {
		defer close(r.done)
		m.runNode(runCtx, r, cred)
	}()
	if cred.Mode == tailnettypes.ModeOAuth {
		go m.checkCredential(runCtx, gen, cred)
	}
}

// updateCredential keeps a connected node when only the credential changes: the retained node needs no
// enrollment, so a new credential only re-validates provisioning (the same mode keeps the node's tags).
func (m *Manager) updateCredential(enabled bool, cred Credential) bool {
	m.mu.Lock()
	r := m.run
	if r == nil || r.ctx == nil || !m.enabled || !enabled || !cred.configured() || cred.Mode != m.cred.Mode || m.conn.State != "connected" {
		m.mu.Unlock()
		return false
	}
	m.cred = cred
	m.credCounter++
	m.provApplied = m.provSeq
	m.prov = initialProvisioning(true, cred)
	gen := m.gen
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
	if cred.Mode == tailnettypes.ModeOAuth {
		go m.checkCredential(r.ctx, gen, cred)
	}
	return true
}

func initialProvisioning(enabled bool, cred Credential) tailnettypes.Provisioning {
	switch {
	case cred.Mode == tailnettypes.ModeManual:
		return tailnettypes.Provisioning{State: "not-applicable"}
	case !cred.configured():
		return tailnettypes.Provisioning{State: "not-configured"}
	case enabled:
		return tailnettypes.Provisioning{State: "checking"}
	}
	return tailnettypes.Provisioning{State: "not-configured"}
}

func stopRun(ctx context.Context, r *run) {
	if r == nil {
		return
	}
	r.cancel()
	r.ingress.stop(ctx)
	if r.listener != nil {
		_ = r.listener.Close()
	}
	<-r.done
}

// Reset disables the route and deletes only the hub's local node state; the device stays in the tailnet admin.
func (m *Manager) Reset(ctx context.Context) error {
	m.mu.Lock()
	cred := m.cred
	m.mu.Unlock()
	m.Apply(ctx, false, cred)
	return os.RemoveAll(m.cfg.StateDir)
}

// Close stops the node at process shutdown and keeps its state.
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	m.gen++
	old := m.run
	m.run = nil
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
	stopRun(ctx, old)
}

func (m *Manager) setConnection(gen int64, conn tailnettypes.Connection) {
	m.mu.Lock()
	if m.gen != gen {
		m.mu.Unlock()
		return
	}
	m.conn = conn
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
}

// current reports whether gen and cred are still the desired state.
func (m *Manager) current(gen int64, cred Credential) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gen == gen && m.cred == cred
}

// beginProvisioning numbers a provisioning operation; only a newer operation's result may replace a recorded one.
func (m *Manager) beginProvisioning() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provSeq++
	return m.provSeq
}

func (m *Manager) setProvisioning(gen, seq int64, cred Credential, prov tailnettypes.Provisioning) {
	m.mu.Lock()
	if m.gen != gen || m.cred != cred || seq <= m.provApplied {
		m.mu.Unlock()
		return
	}
	m.provApplied = seq
	m.prov = prov
	fns := m.notifyLocked()
	m.mu.Unlock()
	fire(fns)
}

// checkCredential validates the OAuth credential; a provider failure without a refusal is recorded as
// unavailable and retried with backoff until the credential is accepted or refused, or the generation ends.
func (m *Manager) checkCredential(ctx context.Context, gen int64, cred Credential) {
	delay := m.cfg.ProvisionRetryMin
	for {
		seq := m.beginProvisioning()
		checkCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		_, err := m.cfg.Provisioner.AccessToken(checkCtx, cred.ClientID, cred.ClientSecret)
		cancel()
		if m.recordProvisioning(gen, seq, cred, err, true) || ctx.Err() != nil || !m.current(gen, cred) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, m.cfg.ProvisionRetryMax)
	}
}

// recordProvisioning maps a provider result and reports whether it is final (accepted or refused).
// Only a credential refusal marks provisioning rejected; other failures of a check end checking as
// unavailable, while a failed mint leaves the recorded state to the check (the request reports its error).
func (m *Manager) recordProvisioning(gen, seq int64, cred Credential, err error, check bool) bool {
	var problem *tailnettypes.Problem
	switch {
	case err == nil:
		m.setProvisioning(gen, seq, cred, tailnettypes.Provisioning{State: "ok"})
		return true
	case errors.As(err, &problem) && (problem.Code == tailnettypes.CodeCredentialRejected || problem.Code == tailnettypes.CodeTagsNotPermitted):
		m.setProvisioning(gen, seq, cred, tailnettypes.Provisioning{State: "rejected", Code: problem.Code, ProviderStatus: problem.ProviderStatus, RefusedTags: problem.RefusedTags})
		return true
	}
	slog.Debug("Tailnet provisioning failed without a refusal", "error", redactSecrets(err.Error(), cred.secrets()...))
	if check {
		prov := tailnettypes.Provisioning{State: "unavailable", Code: tailnettypes.CodeProviderUnavailable}
		if errors.As(err, &problem) {
			prov.ProviderStatus = problem.ProviderStatus
		}
		m.setProvisioning(gen, seq, cred, prov)
	}
	return false
}

func (m *Manager) runNode(ctx context.Context, r *run, cred Credential) {
	delay := m.cfg.RetryMin
	var tags []string
	if cred.Mode == tailnettypes.ModeOAuth {
		tags = []string{HubTag}
	}
	// A retained identity starts without the credential, so enrollment problems cannot stop a registered node;
	// the credential is used only for a first join or when the retained node needs a new login.
	enroll := !HasNodeState(m.cfg.StateDir)
	for {
		authKey := ""
		if enroll {
			authKey = HubAuthKey(cred.Mode, cred.ClientSecret, cred.AuthKey)
		}
		node, err := m.cfg.Factory(NodeConfig{
			Dir:           m.cfg.StateDir,
			Hostname:      m.cfg.Hostname,
			ControlURL:    m.cfg.ControlURL,
			AuthKey:       authKey,
			AdvertiseTags: tags,
			Secrets:       cred.secrets(),
		})
		if err == nil {
			upCtx, cancel := context.WithTimeout(ctx, m.cfg.JoinTimeout)
			err = node.Up(upCtx)
			cancel()
			if err == nil {
				m.serve(ctx, r, node)
				_ = node.Close()
				return
			}
			if !enroll && needsLogin(ctx, node) {
				enroll = true
			}
			_ = node.Close()
		}
		if ctx.Err() != nil {
			return
		}
		if problem := joinRejected(err); problem != nil {
			slog.Warn("Tailnet credential rejected", "code", problem.Code)
			m.setConnection(r.gen, tailnettypes.Connection{State: "error", Code: problem.Code, RefusedTags: problem.RefusedTags})
			if cred.Mode == tailnettypes.ModeOAuth {
				m.setProvisioning(r.gen, m.beginProvisioning(), cred, tailnettypes.Provisioning{State: "rejected", Code: problem.Code, ProviderStatus: problem.ProviderStatus, RefusedTags: problem.RefusedTags})
			}
			return
		}
		slog.Info("Tailnet join failed; retrying", "code", tailnettypes.CodeCoordinationUnreachable, "retryIn", delay, "enroll", enroll)
		m.setConnection(r.gen, tailnettypes.Connection{State: "joining", Code: tailnettypes.CodeCoordinationUnreachable})
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, m.cfg.RetryMax)
	}
}

// needsLogin reports whether a node that failed to come up waits for a new login (expired or removed identity).
func needsLogin(ctx context.Context, node Node) bool {
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := node.Status(statusCtx)
	return err == nil && status != nil && (status.BackendState == ipn.NeedsLogin.String() || status.BackendState == ipn.NeedsMachineAuth.String())
}

// joinRejected recognises control refusing the key (backend error) or the OAuth exchange refusing the secret.
func joinRejected(err error) *tailnettypes.Problem { return joinRefusal(err) }

func (m *Manager) serve(ctx context.Context, r *run, node Node) {
	listener, err := node.Listen("tcp", ":"+IngressPort)
	if err != nil {
		slog.Warn("Tailnet ingress listener failed", "error", err)
		m.setConnection(r.gen, tailnettypes.Connection{State: "error", Code: tailnettypes.CodeNotConnected})
		return
	}
	m.mu.Lock()
	if m.gen != r.gen {
		m.mu.Unlock()
		_ = listener.Close()
		return
	}
	r.node = node
	r.listener = listener
	if m.handler != nil {
		r.ingress = startIngress(listener, m.handler)
	}
	m.mu.Unlock()
	m.watch(ctx, r.gen, node)
}

func newBootID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic("tailnet boot identity unavailable")
	}
	return hex.EncodeToString(id[:])
}

func (m *Manager) credentialRevisionLocked() string {
	return m.bootID + ":" + strconv.FormatUint(m.credCounter, 10)
}
