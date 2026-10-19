// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package edge

import (
 "github.com/cenkalti/backoff/v5"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/coder/websocket"
	"github.com/samber/mo"
	kit "go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/concurrency"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

const (
	// maxReconnectInterval caps the exponential reconnect backoff so a long-dead
	// manager is still retried at a sane cadence.
	maxReconnectInterval = 60 * time.Second
	// healthyTunnelSessionDuration is how long a tunnel session must survive
	// before the reconnect backoff is treated as recovered.
	healthyTunnelSessionDuration = 60 * time.Second
	// maxPollRetryInterval caps the exponential backoff applied to failed poll
	// control requests. Successful polls return to the manager-advertised interval.
	maxPollRetryInterval            = 60 * time.Second
	tunnelSessionWorkerDrainTimeout = 10 * time.Second

	// DefaultHeartbeatInterval is how often the client sends heartbeats
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultWriteTimeout is the timeout for write operations
	DefaultWriteTimeout = 10 * time.Second
	// DefaultRequestTimeout is the timeout for executing local requests
	DefaultRequestTimeout = 5 * time.Minute
	// DefaultGRPCRegistrationTimeout bounds how long the agent waits for the
	// manager to acknowledge tunnel registration on any transport before
	// treating it as a failed transport attempt.
	DefaultGRPCRegistrationTimeout = 10 * time.Second
	// DefaultWebSocketPreferenceTTL keeps websocket as the preferred transport
	// for a short period after a successful auto-mode fallback.
	DefaultWebSocketPreferenceTTL = 2 * time.Minute
	// maxWebSocketPreferenceTTL caps the exponential backoff applied to the
	// websocket preference window while gRPC keeps failing, so a persistently
	// broken gRPC path is retried at most this often but never disabled.
	maxWebSocketPreferenceTTL = 30 * time.Minute
	defaultCommandChunkSize   = 256 * 1024

	// errTunnelRegistrationTimeout marks a registration attempt where the manager
	// accepted the connection but never answered the register message.

	// errEstablishedTunnelSessionEnded marks a session that the manager accepted
	// and later dropped, as opposed to a transport that never connected.

)

var (
	errTunnelRegistrationTimeout = errors.New("timed out waiting for tunnel registration response")

	errEstablishedTunnelSessionEnded = errors.New("established edge tunnel session ended")
)

func (t *commandRequestTransfer) stopInternal() {
	if t == nil {
		return
	}
	t.timerMu.Lock()
	defer t.timerMu.Unlock()
	if t.timer != nil {
		t.timer.Stop()
	}
}

// NewTunnelClient creates a new tunnel client
func NewTunnelClient(cfg *Config, handler http.Handler) *TunnelClient {
	reconnectInterval := time.Duration(cfg.EdgeReconnectInterval) * time.Second
	if reconnectInterval < time.Second {
		reconnectInterval = 5 * time.Second
	}

	managerURL := ""
	if managerBaseURL := strings.TrimRight(httpx.ManagerBaseURL(cfg.ManagerApiUrl), "/"); managerBaseURL != "" {
		// Convert HTTP to WebSocket URL
		managerURL = HTTPToWebSocketURL(managerBaseURL) + "/api/tunnel/connect"
	}
	managerGRPCAddr := httpx.ManagerGRPCAddr(cfg.ManagerApiUrl)

	// Get local port for WebSocket dialing
	localPort := cfg.Port
	if localPort == "" {
		localPort = "3552" // Default port
	}

	return &TunnelClient{
		cfg:                    cfg,
		handler:                handler,
		reconnectInterval:      reconnectInterval,
		heartbeatInterval:      DefaultHeartbeatInterval,
		registrationTimeout:    DefaultGRPCRegistrationTimeout,
		websocketPreferenceTTL: DefaultWebSocketPreferenceTTL,
		healthySessionDuration: healthyTunnelSessionDuration,
		managerURL:             managerURL,
		managerGRPCAddr:        managerGRPCAddr,
		localPort:              localPort,
		requestTimeout:         DefaultRequestTimeout,
		agentInstanceID:        uuid.New().String(),
	}
}

// StartWithErrorChan runs the tunnel client and optionally emits connection errors.
func (c *TunnelClient) StartWithErrorChan(ctx context.Context, errCh chan error) {
	if errCh != nil {
		defer close(errCh)
	}
	slog.InfoContext(ctx, "Starting edge agent session client", StartupLogAttrs(c.cfg)...)
	// A fixed reconnect interval meant a permanently broken or unauthorized agent
	// hammered the manager's auth and DB path forever. Back off exponentially
	// (with jitter) up to maxReconnectInterval, and reset only once a session has
	// stayed up long enough to count as healthy.
	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.InitialInterval = c.reconnectInterval
	reconnectBackoff.MaxInterval = maxReconnectInterval

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "Edge tunnel client shutting down")
			return
		default:
			sessionStart := time.Now()
			if err := c.connectAndServe(ctx); err != nil {
				slog.WarnContext(ctx, "Edge tunnel disconnected", "error", err)
				if errCh != nil {
					select {
					case errCh <- err:
					default:
					}
				}
			}

			if time.Since(sessionStart) >= healthyTunnelSessionDuration {
				reconnectBackoff.Reset()
			}

			// Wait before reconnecting
			delay := reconnectBackoff.NextBackOff()
			reconnectTimer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				reconnectTimer.Stop()
				return
			case <-reconnectTimer.C:
				reconnectTimer.Stop()
				slog.InfoContext(ctx, "Attempting to reconnect edge tunnel", "delay", delay)
			}
		}
	}
}

func StartupLogAttrs(cfg *Config) []any {
	if cfg == nil {
		return []any{
			"control_plane", "unknown",
			"managed_session_transports",
			[]string{},
			"security_mode", NormalizeEdgeMTLSMode(""),
		}
	}

	controlPlane := kit.Ternary(UsePollEdgeTransport(cfg), EdgeTransportPoll, "managed")

	managedSessionTransports := make([]string, 0, 2)
	if UseGRPCEdgeTransport(cfg) || (UsePollEdgeTransport(cfg) && strings.TrimSpace(httpx.ManagerGRPCAddr(cfg.ManagerApiUrl)) != "") {
		managedSessionTransports = append(managedSessionTransports, EdgeTransportGRPC)
	}
	if UseWebSocketEdgeTransport(cfg) || (UsePollEdgeTransport(cfg) && strings.TrimSpace(httpx.ManagerBaseURL(cfg.ManagerApiUrl)) != "") {
		managedSessionTransports = append(managedSessionTransports, EdgeTransportWebSocket)
	}

	attrs := []any{
		"control_plane", controlPlane,
		"managed_session_transports", managedSessionTransports,
		"security_mode", NormalizeEdgeMTLSMode(cfg.EdgeMTLSMode),
	}

	if managerAPIURL := strings.TrimSpace(cfg.ManagerApiUrl); managerAPIURL != "" {
		attrs = append(attrs, "manager_api_url", managerAPIURL)
	}
	if managerGRPCAddr := strings.TrimSpace(httpx.ManagerGRPCAddr(cfg.ManagerApiUrl)); managerGRPCAddr != "" {
		attrs = append(attrs, "manager_grpc_addr", managerGRPCAddr)
	}
	if managerBaseURL := strings.TrimSpace(httpx.ManagerBaseURL(cfg.ManagerApiUrl)); managerBaseURL != "" {
		attrs = append(attrs, "manager_base_url", managerBaseURL)
	}

	return attrs
}

// connectAndServe establishes a connection and handles messages.
func (c *TunnelClient) connectAndServe(ctx context.Context) error {
	if UsePollEdgeTransport(c.cfg) {
		return c.connectAndServePoll(ctx)
	}
	return c.connectAndServeManagedTunnelInternal(ctx)
}

func (c *TunnelClient) connectAndServeManagedTunnelInternal(ctx context.Context) error {
	transports := c.managedTunnelTransportsInternal()
	if transports.grpc {
		if preferredUntil, ok := c.preferredWebSocketUntilInternal(time.Now()).Get(); ok {
			slog.InfoContext(ctx, "Temporarily preferring websocket edge tunnel transport after recent websocket success",
				"preferredUntil", preferredUntil,
				"grpcFailureStreak", c.grpcFailureStreakInternal(),
				"managerWsUrl", c.managerWebSocketURLInternal(),
			)
			return c.connectAndServeWebSocketInternal(ctx)
		}

		sessionStart := time.Now()
		if err := c.connectAndServeGRPC(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A healthy gRPC session that the manager dropped (restart, redeploy)
			// is not a broken gRPC path: let the outer loop back off and retry
			// gRPC first instead of racing a websocket dial against the restart.
			if errors.Is(err, errEstablishedTunnelSessionEnded) && time.Since(sessionStart) >= c.healthySessionDurationInternal() {
				return err
			}
			c.noteGRPCTunnelFailureInternal()
			if transports.websocket {
				managerWSURL := c.managerWebSocketURLInternal()
				slog.WarnContext(ctx, "gRPC edge tunnel connection failed, falling back to websocket transport",
					"error", err,
					"managerGrpcAddr", c.managerGRPCAddr,
					"managerWsUrl", managerWSURL,
				)
				if wsErr := c.connectAndServeWebSocketInternal(ctx); wsErr != nil {
					// Keep both transport failures in the chain for errors.Is/errors.As traversal.
					return fmt.Errorf("gRPC edge tunnel failed: %w; websocket fallback failed: %w", err, wsErr)
				}
				return nil
			}
			return err
		}
		return nil
	}
	if transports.websocket {
		return c.connectAndServeWebSocketInternal(ctx)
	}
	return errors.New("no edge tunnel transport is available")
}

func (c *TunnelClient) managedTunnelTransportsInternal() managedTunnelTransportsInternal {
	if c == nil || c.cfg == nil {
		return managedTunnelTransportsInternal{}
	}

	managerGRPCAvailable := strings.TrimSpace(c.managerGRPCAddr) != ""
	managerWebSocketAvailable := c.managerWebSocketURLInternal() != ""
	transport := NormalizeEdgeTransport(c.cfg.EdgeTransport)

	switch transport {
	case EdgeTransportAuto:
		return managedTunnelTransportsInternal{
			grpc:      managerGRPCAvailable,
			websocket: managerWebSocketAvailable,
		}
	case EdgeTransportPoll:
		return managedTunnelTransportsInternal{
			grpc:      managerGRPCAvailable,
			websocket: managerWebSocketAvailable,
		}
	case EdgeTransportWebSocket:
		return managedTunnelTransportsInternal{
			websocket: managerWebSocketAvailable,
		}
	case EdgeTransportGRPC:
		return managedTunnelTransportsInternal{
			grpc: managerGRPCAvailable,
		}
	default:
		return managedTunnelTransportsInternal{}
	}
}

func (c *TunnelClient) managerWebSocketURLInternal() string {
	if c == nil {
		return ""
	}
	if managerURL := strings.TrimSpace(c.managerURL); managerURL != "" {
		return managerURL
	}
	if c.cfg == nil {
		return ""
	}
	managerBaseURL := strings.TrimRight(strings.TrimSpace(httpx.ManagerBaseURL(c.cfg.ManagerApiUrl)), "/")
	if managerBaseURL == "" {
		return ""
	}
	return HTTPToWebSocketURL(managerBaseURL) + "/api/tunnel/connect"
}

func (c *TunnelClient) registrationTimeoutInternal() time.Duration {
	if c == nil || c.registrationTimeout <= 0 {
		return DefaultGRPCRegistrationTimeout
	}
	return c.registrationTimeout
}

func (c *TunnelClient) requestTimeoutInternal() time.Duration {
	if c == nil || c.requestTimeout <= 0 {
		return DefaultRequestTimeout
	}
	return c.requestTimeout
}

func (c *TunnelClient) healthySessionDurationInternal() time.Duration {
	if c == nil || c.healthySessionDuration <= 0 {
		return healthyTunnelSessionDuration
	}
	return c.healthySessionDuration
}

func (c *TunnelClient) websocketPreferenceTTLInternal() time.Duration {
	if c == nil || c.websocketPreferenceTTL <= 0 {
		return DefaultWebSocketPreferenceTTL
	}
	return c.websocketPreferenceTTL
}

func (c *TunnelClient) preferredWebSocketUntilInternal(now time.Time) mo.Option[time.Time] {
	// A nil client yields a zero transports struct, so this also guards nil.
	transports := c.managedTunnelTransportsInternal()
	if !transports.grpc || !transports.websocket {
		return mo.None[time.Time]()
	}

	c.transportPreferenceMu.RLock()
	defer c.transportPreferenceMu.RUnlock()

	if c.preferWebSocketUntil.IsZero() || !now.Before(c.preferWebSocketUntil) {
		return mo.None[time.Time]()
	}

	return mo.Some(c.preferWebSocketUntil)
}

func (c *TunnelClient) markTransportConnectedInternal(transport string) {
	if c == nil {
		return
	}

	c.transportPreferenceMu.Lock()
	defer c.transportPreferenceMu.Unlock()

	switch transport {
	case EdgeTransportGRPC:
		c.preferWebSocketUntil = time.Time{}
		c.grpcFailureStreak = 0
	case EdgeTransportWebSocket:
		if transports := c.managedTunnelTransportsInternal(); transports.grpc && transports.websocket {
			// Back the preference window off exponentially while gRPC keeps
			// failing so each reconnect does not re-pay the registration
			// timeout against a persistently broken gRPC path.
			ttl := c.websocketPreferenceTTLInternal()
			if streak := min(c.grpcFailureStreak, 4); streak > 0 {
				ttl = min(ttl<<streak, maxWebSocketPreferenceTTL)
			}
			c.preferWebSocketUntil = time.Now().Add(ttl)
		}
	}
}

func (c *TunnelClient) noteGRPCTunnelFailureInternal() {
	if c == nil {
		return
	}
	c.transportPreferenceMu.Lock()
	defer c.transportPreferenceMu.Unlock()
	c.grpcFailureStreak++
}

func (c *TunnelClient) grpcFailureStreakInternal() int {
	if c == nil {
		return 0
	}
	c.transportPreferenceMu.RLock()
	defer c.transportPreferenceMu.RUnlock()
	return c.grpcFailureStreak
}

func (c *TunnelClient) registerMessageInternal() *TunnelMessage {
	capabilities := AdvertisedEdgeCommands()
	capabilities = append(capabilities, tunnelCapabilityChunkedRequest, tunnelCapabilityProtoParity)
	registration, _ := c.registration.Load()
	return &TunnelMessage{
		Type:          MessageTypeRegister,
		AgentToken:    c.cfg.AgentToken,
		AgentInstance: c.agentInstanceID,
		Capabilities:  capabilities,
		ResumeSession: registration.sessionID,
	}
}

func (c *TunnelClient) awaitRegistrationInternal(ctx context.Context, conn TunnelConnection) (*TunnelMessage, error) {
	if c == nil {
		return nil, errors.New("edge tunnel connection is not initialized")
	}
	if conn == nil {
		return nil, errors.New("edge tunnel connection is not initialized")
	}

	type registrationResult struct {
		msg *TunnelMessage
		err error
	}

	timeout := c.registrationTimeoutInternal()
	recvCh := make(chan registrationResult, 1)
	go func() {
		msg, err := conn.Receive()
		recvCh <- registrationResult{msg: msg, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	case <-timer.C:
		_ = conn.Close()
		return nil, fmt.Errorf("after %s: %w", timeout, errTunnelRegistrationTimeout)
	case result := <-recvCh:
		if result.err != nil {
			return nil, fmt.Errorf("failed to receive tunnel registration response: %w", result.err)
		}
		if result.msg == nil {
			return nil, errors.New("received empty tunnel registration response")
		}
		if result.msg.Type != MessageTypeRegisterResponse {
			return nil, fmt.Errorf("unexpected first tunnel message: %s", result.msg.Type)
		}
		if !result.msg.Accepted {
			return nil, fmt.Errorf("manager rejected tunnel registration: %s", result.msg.Error)
		}
		c.registration.Store(clientRegistrationInternal{sessionID: result.msg.SessionID})
		return result.msg, nil
	}
}

// serveTunnelSessionInternal runs the shared register/heartbeat/message
// lifecycle on an established tunnel connection, for every transport.
func (c *TunnelClient) serveTunnelSessionInternal(ctx context.Context, conn TunnelConnection, managerAddr string) error {
	box := &connBox{conn: conn}
	c.conn.Store(box)
	setActiveAgentTunnelConn(conn)
	connCtx, connCancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	stopCloseWatch := context.AfterFunc(connCtx, func() { _ = conn.Close() })
	defer func() {
		connCancel()
		c.cancelRuntimeSessionInternal(conn)
		c.closeAllStreams()
		c.clearCommandRequestTransfersInternal(conn)
		_ = conn.Close()
		stopCloseWatch()
		drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(ctx), tunnelSessionWorkerDrainTimeout)
		if err := utils.WaitGroup(drainCtx, &workers); err != nil {
			slog.WarnContext(context.WithoutCancel(ctx), "Timed out waiting for edge tunnel session workers", "error", err)
		}
		cancelDrain()
		c.conn.CompareAndSwap(box, nil)
		clearActiveAgentTunnelConn(conn)
	}()

	if err := conn.Send(c.registerMessageInternal()); err != nil {
		// A rejected gRPC stream can report EOF from Send; Recv carries the RPC status.
		if conn.Transport() != EdgeTransportGRPC || !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to send %s tunnel register message: %w", conn.Transport(), err)
		}
	}

	registerMsg, err := c.awaitRegistrationInternal(ctx, conn)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "Edge tunnel connected to manager",
		"transport", conn.Transport(),
		"managerAddr", managerAddr,
		"environmentId", registerMsg.EnvironmentID,
		"sessionId", registerMsg.SessionID,
	)
	c.markTransportConnectedInternal(conn.Transport())

	workers.Go(func() { c.heartbeatLoop(connCtx, conn) })

	if messageLoopErr := c.messageLoop(connCtx, conn, &workers); messageLoopErr != nil {
		return fmt.Errorf("%w: %w", errEstablishedTunnelSessionEnded, messageLoopErr)
	}
	return nil
}

// heartbeatLoop sends periodic heartbeats
func (c *TunnelClient) heartbeatLoop(ctx context.Context, conn TunnelConnection) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if conn == nil || conn.IsClosed() {
				return
			}

			msg := &TunnelMessage{
				ID:   uuid.New().String(),
				Type: MessageTypeHeartbeat,
			}

			if err := conn.Send(msg); err != nil {
				slog.WarnContext(ctx, "Failed to send heartbeat", "error", err)
				// Force reconnect so the manager does not keep stale state without heartbeats.
				if closeErr := conn.Close(); closeErr != nil {
					slog.DebugContext(ctx, "Failed to close tunnel connection after heartbeat failure", "error", closeErr)
				}
				return
			}
			slog.DebugContext(ctx, "Sent heartbeat to manager")
		}
	}
}

// agentAuthCredentialsInternal returns the canonical header/metadata credential
// set every agent transport presents, so the three transports cannot drift.
func agentAuthCredentialsInternal(token string) map[string]string {
	return map[string]string{
		HeaderAgentToken:    token,
		HeaderAPIKey:        token,
		HeaderAuthorization: "Bearer " + token,
	}
}

func StartTunnelClient(ctx context.Context, cfg *Config, handler http.Handler) (func(context.Context) error, error) {
	if !cfg.EdgeAgent {
		return nil, errors.New("edge tunnel disabled")
	}

	if UseGRPCEdgeTransport(cfg) {
		if httpx.ManagerGRPCAddr(cfg.ManagerApiUrl) == "" {
			return nil, errors.New("MANAGER_API_URL with a valid host is required for gRPC transport")
		}
	}

	if UseWebSocketEdgeTransport(cfg) && strings.TrimSpace(httpx.ManagerBaseURL(cfg.ManagerApiUrl)) == "" {
		return nil, errors.New("MANAGER_API_URL is required for websocket transport")
	}

	if UsePollEdgeTransport(cfg) && strings.TrimSpace(httpx.ManagerBaseURL(cfg.ManagerApiUrl)) == "" {
		return nil, errors.New("MANAGER_API_URL is required for poll transport")
	}

	if cfg.AgentToken == "" {
		return nil, errors.New("AGENT_TOKEN is required")
	}

	if err := EnsureAgentMTLSAssets(ctx, cfg); err != nil {
		return nil, err
	}
	if err := ValidateAgentMTLSConfig(cfg); err != nil {
		return nil, err
	}

	client := NewTunnelClient(cfg, handler)
	return concurrency.StartSupervised(ctx, "Edge tunnel client", func(runCtx context.Context) error {
		client.StartWithErrorChan(runCtx, nil)
		return nil
	})
}

func (c *TunnelClient) connectAndServeWebSocketInternal(ctx context.Context) error {
	managerWSURL := c.managerWebSocketURLInternal()
	if managerWSURL == "" {
		return errors.New("manager WebSocket URL is empty")
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if strings.HasPrefix(strings.ToLower(managerWSURL), "wss://") {
		tlsConfig, err := buildManagerClientTLSConfigInternal(c.cfg)
		if err != nil {
			return fmt.Errorf("failed to configure edge websocket TLS: %w", err)
		}
		if tlsConfig == nil {
			tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		transport.TLSClientConfig = tlsConfig
	}

	headers := http.Header{}
	for header, value := range agentAuthCredentialsInternal(c.cfg.AgentToken) {
		headers.Set(header, value)
	}

	slog.DebugContext(ctx, "Dialing manager for websocket edge tunnel", "url", managerWSURL)

	// The dial context bounds only the handshake; the connection outlives it.
	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	conn, resp, err := websocket.Dial(dialCtx, managerWSURL, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: transport},
		HTTPHeader: headers,
	})
	if err != nil {
		if resp != nil && resp.Body != nil {
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to connect to manager websocket endpoint (status: %d, body: %s): %w", resp.StatusCode, string(body), err)
		}
		return fmt.Errorf("failed to connect to manager websocket endpoint: %w", err)
	}

	return c.serveTunnelSessionInternal(ctx, NewTunnelConn(conn), managerWSURL)
}
