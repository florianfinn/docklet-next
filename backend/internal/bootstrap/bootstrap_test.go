// Source: getarcaneapp/arcane/backend/internal/bootstrap/bootstrap_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm.

package bootstrap

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libcrypto "go.getarcane.app/sys/crypto"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
)

func TestNormalizeTunnelGRPCRequestPathInternal(t *testing.T) {
	fullMethodPath := tunnelpb.TunnelService_Connect_FullMethodName

	t.Run("nil request", func(t *testing.T) {
		assert.Nil(t, normalizeTunnelGRPCRequestPathInternal(nil))
	})

	t.Run("path without prefix remains unchanged", func(t *testing.T) {
		req := httptest.NewRequest("POST", fullMethodPath, http.NoBody)
		normalized := normalizeTunnelGRPCRequestPathInternal(req)

		assert.Same(t, req, normalized)
		assert.Equal(t, fullMethodPath, normalized.URL.Path)
	})

	t.Run("api prefix is removed", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api"+fullMethodPath, http.NoBody)
		normalized := normalizeTunnelGRPCRequestPathInternal(req)

		assert.NotSame(t, req, normalized)
		assert.Equal(t, fullMethodPath, normalized.URL.Path)
		assert.Equal(t, fullMethodPath, normalized.RequestURI)
	})

	t.Run("nested proxy prefix is removed up to method path", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/edge/proxy/api"+fullMethodPath, http.NoBody)
		normalized := normalizeTunnelGRPCRequestPathInternal(req)

		assert.NotSame(t, req, normalized)
		assert.Equal(t, fullMethodPath, normalized.URL.Path)
		assert.Equal(t, fullMethodPath, normalized.RequestURI)
	})

	t.Run("legacy /api/tunnel/connect is rewritten to gRPC method", func(t *testing.T) {
		// Regression: PR #2722 removed this branch, breaking the edge agent's
		// gRPC transport. The agent client uses /api/tunnel/connect as its
		// gRPC method path so reverse proxies can route tunnel traffic with
		// a stable URL instead of the proto-generated gRPC service name.
		req := httptest.NewRequest("POST", "/api/tunnel/connect", http.NoBody)
		normalized := normalizeTunnelGRPCRequestPathInternal(req)

		assert.NotSame(t, req, normalized)
		assert.Equal(t, fullMethodPath, normalized.URL.Path)
		assert.Equal(t, fullMethodPath, normalized.RequestURI)
	})

	t.Run("nested proxy with legacy /api/tunnel/connect is rewritten", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/edge/proxy/api/tunnel/connect", http.NoBody)
		normalized := normalizeTunnelGRPCRequestPathInternal(req)

		assert.NotSame(t, req, normalized)
		assert.Equal(t, fullMethodPath, normalized.URL.Path)
		assert.Equal(t, fullMethodPath, normalized.RequestURI)
	})
}

func TestIsTunnelGRPCRequestInternal(t *testing.T) {
	fullMethodPath := tunnelpb.TunnelService_Connect_FullMethodName

	t.Run("detects by grpc content-type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/any/path", http.NoBody)
		req.Header.Set("Content-Type", "application/grpc")
		assert.True(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("detects by grpc-web content-type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/any/path", http.NoBody)
		req.Header.Set("Content-Type", "application/grpc-web+proto")
		assert.True(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("detects by method path without grpc content-type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, fullMethodPath, http.NoBody)
		assert.True(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("does not match regular api requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/environments/pair", http.NoBody)
		req.Header.Set("Content-Type", "application/json")
		assert.False(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("requires post", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fullMethodPath, http.NoBody)
		req.Header.Set("Content-Type", "application/grpc")
		assert.False(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("does not match http2 post with te trailers and json content-type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", http.NoBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Te", "trailers")
		req.ProtoMajor = 2
		assert.False(t, isTunnelGRPCRequestInternal(req))
	})

	t.Run("does not match http2 post with te trailers and form content-type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/logout", http.NoBody)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Te", "trailers")
		req.ProtoMajor = 2
		assert.False(t, isTunnelGRPCRequestInternal(req))
	})
}

func TestConfigureHTTPProtocolsInternal(t *testing.T) {
	handler := http.NewServeMux()

	t.Run("tls enables http1 and http2", func(t *testing.T) {
		configuredHandler, protocols := configureHTTPProtocolsInternal(true, handler)

		assert.Same(t, handler, configuredHandler)
		require.NotNil(t, protocols)
		assert.True(t, protocols.HTTP1())
		assert.True(t, protocols.HTTP2())
		assert.False(t, protocols.UnencryptedHTTP2())
	})

	t.Run("plain enables http1 and unencrypted http2", func(t *testing.T) {
		configuredHandler, protocols := configureHTTPProtocolsInternal(false, handler)

		// The plain path wraps the handler to clear the stale h2c read
		// deadline, so it is deliberately not the same handler.
		assert.NotNil(t, configuredHandler)
		require.NotNil(t, protocols)
		assert.True(t, protocols.HTTP1())
		assert.False(t, protocols.HTTP2())
		assert.True(t, protocols.UnencryptedHTTP2())
	})
}

func TestHTTP2APIResponsesDoNotUseAPIGzipInternal(t *testing.T) {
	cfg := &config.Config{
		AgentMode:   true,
		AppUrl:      "http://localhost:3552",
		Environment: config.AppEnvironmentTest,
	}
	router, _ := newRouter(RouterParams{
		Context: t.Context(),
		Config:  cfg,
		HandlerDeps: api.HandlerDeps{
			Project:   project.New(&project.ProjectService{}, nil),
			Container: container.New(&container.ContainerService{}, nil, nil, nil),
			System:    system.New(&system.SystemService{}, nil, nil, nil, nil),
		},
		AuthMiddleware: auth.NewAuthMiddleware(nil, cfg),
		TunnelRegistry: edge.NewTunnelRegistry(),
	})
	const largePath = "/api/test-large-response"
	router.GET(largePath, func(c *echo.Context) error {
		return c.String(http.StatusOK, strings.Repeat("agent-response-", 1024))
	})
	handler, protocols := configureHTTPProtocolsInternal(false, router)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, server.Shutdown(context.WithoutCancel(t.Context())))
		require.ErrorIs(t, <-errCh, http.ErrServerClosed)
	})

	clientProtocols := &http.Protocols{}
	clientProtocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: clientProtocols, DisableCompression: true}
	client := &http.Client{Transport: transport}

	for _, route := range []struct {
		path   string
		status int
	}{{"/api/health", http.StatusOK}, {"/api/openapi.json", http.StatusNotFound}, {largePath, http.StatusOK}} {
		t.Run(route.path, func(t *testing.T) {
			req, requestErr := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+route.path, http.NoBody)
			require.NoError(t, requestErr)
			req.Header.Set("Accept-Encoding", "gzip")

			resp, responseErr := client.Do(req)
			require.NoError(t, responseErr)
			defer func() { _ = resp.Body.Close() }()

			body, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			require.Equal(t, "HTTP/2.0", resp.Proto)
			require.Equal(t, route.status, resp.StatusCode)
			require.Empty(t, resp.Header.Get("Content-Encoding"))
			if route.path == largePath {
				require.Equal(t, strings.Repeat("agent-response-", 1024), string(body))
			}

			if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
				parsedLength, parseErr := strconv.Atoi(contentLength)
				require.NoError(t, parseErr)
				require.Equal(t, len(body), parsedLength)
			}
		})
	}
}

func TestH2CStreamSurvivesPastReadHeaderTimeoutInternal(t *testing.T) {
	// Regression guard for the go1.26.6 CVE-2026-56853 backport, which arms
	// ReadHeaderTimeout on the raw conn before the h2c preface sniff and never
	// clears it on the HTTP/2 handoff. Without ClearReadDeadline in the h2c
	// handler wrapper, the connection — and every gRPC tunnel stream on it —
	// dies with an i/o timeout exactly ReadHeaderTimeout after accept.
	const readHeaderTimeout = 250 * time.Millisecond

	release := make(chan struct{})
	streamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write([]byte("done"))
	})
	handler, protocols := configureHTTPProtocolsInternal(false, streamHandler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: readHeaderTimeout,
		ConnContext:       httpx.WithConn,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, server.Shutdown(context.WithoutCancel(t.Context())))
		require.ErrorIs(t, <-errCh, http.ErrServerClosed)
	})

	clientProtocols := &http.Protocols{}
	clientProtocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: clientProtocols}
	client := &http.Client{Transport: transport}

	resp, err := client.Get("http://" + listener.Addr().String() + "/stream")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, "HTTP/2.0", resp.Proto)

	// Keep the stream idle well past ReadHeaderTimeout before the handler
	// writes again; on a connection with the stale deadline still armed the
	// read below fails instead of returning the body.
	time.Sleep(4 * readHeaderTimeout)
	close(release)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "done", string(body))
}

func TestHTTPServerStopCancelsStreamingRequestContextsInternal(t *testing.T) {
	appCtx := t.Context()

	handlerEntered := make(chan struct{})
	router := echo.New()
	router.GET("/stream", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Type", "application/x-json-stream")
		c.Response().WriteHeader(http.StatusOK)
		c.Response().(http.Flusher).Flush()
		close(handlerEntered)
		<-c.Request().Context().Done()
		return nil
	})

	cfg := &config.Config{
		AgentMode: true,
		Listen:    "127.0.0.1",
		Port:      "0",
	}
	lifecycle := fxtest.NewLifecycle(t)
	srv, err := NewHTTPServer(lifecycle, HTTPServerParams{
		AppCtx: appCtx,
		Config: cfg,
		Router: router,
	})
	require.NoError(t, err)
	require.NoError(t, lifecycle.Start(t.Context()))

	resp, err := http.Get("http://" + srv.Addr + "/stream")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	<-handlerEntered

	shutdownCtx, cancelShutdown := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelShutdown()
	start := time.Now()
	require.NoError(t, lifecycle.Stop(shutdownCtx))
	require.Less(t, time.Since(start), time.Second)
}

func TestNewHTTPServerRejectsInvalidTLSCertificateInternal(t *testing.T) {
	cfg := &config.Config{
		AgentMode:   true,
		TLSEnabled:  true,
		TLSCertFile: t.TempDir() + "/missing.crt",
		TLSKeyFile:  t.TempDir() + "/missing.key",
	}

	_, err := NewHTTPServer(fxtest.NewLifecycle(t), HTTPServerParams{
		AppCtx: t.Context(),
		Config: cfg,
		Router: echo.New(),
	})
	require.Error(t, err)
}

func TestRegisterAppCancelHookRunsAfterLaterStopHooks(t *testing.T) {
	appCtx, cancelApp := context.WithCancel(t.Context())
	lifecycle := fxtest.NewLifecycle(t)
	registerAppCancelHook(lifecycle, cancelApp)
	appActiveDuringDependencyStop := false
	lifecycle.Append(fx.Hook{
		OnStop: func(context.Context) error {
			appActiveDuringDependencyStop = appCtx.Err() == nil
			return nil
		},
	})

	lifecycle.RequireStart()
	lifecycle.RequireStop()
	require.True(t, appActiveDuringDependencyStop)
	require.ErrorIs(t, appCtx.Err(), context.Canceled)
}

func TestApplicationOptionsValidate(t *testing.T) {
	appCtx, cancelApp := context.WithCancel(t.Context())
	defer cancelApp()

	err := fx.ValidateApp(applicationOptions(
		appCtx,
		&config.Config{},
		(*database.DB)(nil),
		cancelApp,
	))
	require.NoError(t, err)
}

func TestPrepareServerTLSInternal_AgentModeSkipsManagerMTLSValidation(t *testing.T) {
	cfg := &config.Config{
		AgentMode:     true,
		EdgeMTLSMode:  "required",
		ManagerApiUrl: "https://127.0.0.1:3552",
	}

	useTLS, tlsCertFile, tlsKeyFile, edgeCfg, err := prepareServerTLSInternal(t.Context(), cfg)
	require.NoError(t, err)
	assert.False(t, useTLS)
	assert.Empty(t, tlsCertFile)
	assert.Empty(t, tlsKeyFile)
	require.NotNil(t, edgeCfg)
	assert.Equal(t, "required", edgeCfg.EdgeMTLSMode)
}

func TestPrepareServerTLSInternal_AllowsExternalMTLSTermination(t *testing.T) {
	libcrypto.InitEncryption(&libcrypto.Config{
		EncryptionKey: "test-encryption-key-for-edge-mtls-32bytes-min",
		Environment:   "test",
	})

	assetsDir := t.TempDir()
	cfg := &config.Config{
		TLSEnabled:        false,
		EdgeMTLSMode:      "required",
		EdgeMTLSAssetsDir: assetsDir,
		EncryptionKey:     "test-encryption-key-for-edge-mtls-32bytes-min",
	}

	useTLS, tlsCertFile, tlsKeyFile, edgeCfg, err := prepareServerTLSInternal(t.Context(), cfg)
	require.NoError(t, err)
	assert.False(t, useTLS)
	assert.Empty(t, tlsCertFile)
	assert.Empty(t, tlsKeyFile)
	require.NotNil(t, edgeCfg)
	assert.Equal(t, "required", edgeCfg.EdgeMTLSMode)
	require.FileExists(t, edgeCfg.EdgeMTLSCAFile)
	require.FileExists(t, assetsDir+"/ca.crt")
	require.FileExists(t, assetsDir+"/ca.key")
}

func TestIsWeakProductionEncryptionKeyInternal(t *testing.T) {
	assert.True(t, isWeakProductionEncryptionKeyInternal("short", "production", false))
	assert.False(t, isWeakProductionEncryptionKeyInternal("test-encryption-key-for-edge-mtls-32bytes-min", "production", false))
	assert.False(t, isWeakProductionEncryptionKeyInternal("hex:abc", "production", false))
	assert.False(t, isWeakProductionEncryptionKeyInternal("short", "development", false))
}
