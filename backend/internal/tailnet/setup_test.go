// Source: docklet-next/issues/586
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original real-HTTP setup test, lifecycle and provider secrecy proofs.

package tailnet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/views"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/tailnet"
)

func setupManager(t *testing.T, provider http.HandlerFunc) *Manager {
	t.Helper()
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	m := NewManager(ManagerConfig{StateDir: t.TempDir(), MTLSDisabled: true, Provisioner: &Provisioner{BaseURL: server.URL, Client: server.Client()}})
	m.cred = Credential{Mode: dto.ModeOAuth, ClientID: "example", ClientSecret: strings.Repeat("example-secret-", 3)}
	m.enabled = true
	m.conn.State = "connected"
	tags := views.SliceOf([]string{HubTag})
	m.status = &ipnstate.Status{TailscaleIPs: []netip.Addr{netip.MustParseAddr("192.0.2.12")}, Self: &ipnstate.PeerStatus{Tags: &tags}}
	return m
}

func setupRouter(m *Manager, admin, csrf bool, access ...bool) http.Handler {
	router := echo.New()
	if csrf {
		router.Use(middleware.NewCSRFMiddleware(&config.Config{AppUrl: "https://hub.example.com"}).Add())
	}
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("setup-test", "1"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ps := authz.NewPermissionSet()
		ps.UserID = "example-reader"
		ps.AddGlobal(authz.PermSettingsWrite)
		if admin {
			ps = authz.SudoPermissionSet()
		}
		next(huma.WithValue(ctx, middleware.ContextKeyUserPermissions, ps))
	})
	if len(access) == 0 || access[0] {
		api.UseMiddleware(middleware.AccessMiddleware(api))
	}
	NewModule(newService(m, nil, "example-hub", "")).RegisterRoutes(api)
	return router
}

func setupHTTP(t *testing.T, handler http.Handler, ctx context.Context) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/api/tailnet/test", nil).WithContext(ctx))
	return rec.Code, rec.Body.Bytes()
}

func setupResult(t *testing.T, body []byte) dto.TestResult {
	t.Helper()
	var envelope struct {
		Data dto.TestResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Len(t, envelope.Data.Checks, 6)
	return envelope.Data
}

func setupToken(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]string{"access_token": setupTokenValue()})
}

func TestSetupMintRefusal(t *testing.T) {
	for _, status := range []int{400, 401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var deletes atomic.Int32
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					setupToken(w)
					return
				}
				if r.Method == "DELETE" {
					deletes.Add(1)
				}
				w.WriteHeader(status)
			})
			code, body := setupHTTP(t, setupRouter(m, true, false), t.Context())
			require.Equal(t, 200, code, string(body))
			out := setupResult(t, body)
			require.Equal(t, dto.TestCheck{ID: "agent-key", State: "fail", Code: dto.CodeMintRefused, ProviderStatus: status}, out.Checks[1])
			require.Equal(t, "skipped", out.Checks[2].State)
			require.Zero(t, deletes.Load())
		})
	}
}

func TestSetupPayloadAndCleanup(t *testing.T) {
	for _, status := range []int{200, 204, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var tokens, mints, deletes atomic.Int32
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					tokens.Add(1)
					setupToken(w)
					return
				}
				require.Equal(t, "Bearer "+setupTokenValue(), r.Header.Get("Authorization"))
				if r.Method == "POST" {
					mints.Add(1)
					var p keyRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
					require.EqualValues(t, 300, p.ExpirySeconds)
					require.Equal(t, "Docklet setup test", p.Description)
					require.Equal(t, []string{AgentTag}, p.Capabilities.Devices.Create.Tags)
					require.False(t, p.Capabilities.Devices.Create.Preauthorized)
					require.False(t, p.Capabilities.Devices.Create.Reusable)
					require.False(t, p.Capabilities.Devices.Create.Ephemeral)
					_, _ = io.WriteString(w, `{"id":"example/id ?","key":"`+"tskey-"+"auth-example"+`"}`)
					return
				}
				deletes.Add(1)
				require.Equal(t, "DELETE", r.Method)
				require.Equal(t, "/api/v2/tailnet/-/keys/example%2Fid%20%3F", r.URL.EscapedPath())
				w.WriteHeader(status)
			})
			code, body := setupHTTP(t, setupRouter(m, true, false), t.Context())
			require.Equal(t, 200, code, string(body))
			out := setupResult(t, body)
			require.Equal(t, "pass", out.Checks[1].State)
			if status == 500 {
				require.Equal(t, dto.CodeTestCleanupFailed, out.Checks[2].Code)
				require.Equal(t, "fail", out.Checks[2].State)
			} else {
				require.Equal(t, "pass", out.Checks[2].State)
			}
			require.EqualValues(t, 1, tokens.Load())
			require.EqualValues(t, 1, mints.Load())
			require.EqualValues(t, 1, deletes.Load())
		})
	}
}

func TestSetupCleanupSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var deleted atomic.Int32
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			setupToken(w)
			return
		}
		if r.Method == "POST" {
			_, _ = io.WriteString(w, `{"id":"cancel-key"}`)
			return
		}
		require.Equal(t, "/api/v2/tailnet/-/keys/cancel-key", r.URL.Path)
		deleted.Add(1)
		w.WriteHeader(204)
	})
	// Cancel only after the real HTTP mint response has been consumed.
	m.cfg.Provisioner.Client.Transport = setupRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := http.DefaultTransport.RoundTrip(r)
		if err == nil && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/keys") {
			response.Body = &cancelOnClose{ReadCloser: response.Body, cancel: cancel}
		}
		return response, err
	})
	out, err := m.Test(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.EqualValues(t, 1, deleted.Load())
	require.Equal(t, "pass", out.Checks[2].State)
}

type setupRoundTrip func(*http.Request) (*http.Response, error)

func (f setupRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnClose) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }

func TestSetupProviderTimeouts(t *testing.T) {
	for _, phase := range []string{"token", "mint", "delete"} {
		t.Run(phase, func(t *testing.T) {
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				at := "mint"
				if strings.HasSuffix(r.URL.Path, "/token") {
					at = "token"
				}
				if r.Method == "DELETE" {
					at = "delete"
				}
				if at == phase {
					_, _ = io.Copy(io.Discard, r.Body)
					<-r.Context().Done()
					return
				}
				if at == "token" {
					setupToken(w)
				} else {
					_, _ = io.WriteString(w, `{"id":"timeout-key"}`)
				}
			})
			m.cfg.Provisioner.Client.Timeout = 50 * time.Millisecond
			out, err := m.Test(t.Context())
			require.NoError(t, err)
			switch phase {
			case "token":
				require.Equal(t, dto.CodeProviderUnavailable, out.Checks[0].Code)
				require.Equal(t, "skipped", out.Checks[1].State)
			case "mint":
				require.Equal(t, dto.CodeProviderUnavailable, out.Checks[1].Code)
				require.Equal(t, "unknown", out.Checks[2].State)
			case "delete":
				require.Equal(t, "fail", out.Checks[2].State)
				require.Equal(t, dto.CodeTestCleanupFailed, out.Checks[2].Code)
			}
		})
	}
}

func TestSetupConcurrentRequests(t *testing.T) {
	held, release := make(chan struct{}), make(chan struct{})
	var mints atomic.Int32
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			setupToken(w)
			return
		}
		if r.Method == "POST" {
			if mints.Add(1) == 1 {
				close(held)
				<-release
			}
			_, _ = io.WriteString(w, `{"id":"one-key"}`)
			return
		}
		w.WriteHeader(204)
	})
	router := setupRouter(m, true, false)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	first := make(chan int, 1)
	go func() {
		code, _ := setupHTTP(t, router, t.Context())
		first <- code
	}()
	<-held
	code, body := setupHTTP(t, router, t.Context())
	unblock()
	require.Equal(t, 200, <-first)
	require.Equal(t, 409, code, string(body))
	require.Contains(t, string(body), dto.CodeTestRunning)
	require.EqualValues(t, 1, mints.Load())
}

func TestSetupCredentialRevision(t *testing.T) {
	for _, change := range []string{"same-mode", "reset", "disable"} {
		t.Run(change, func(t *testing.T) {
			held, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					setupToken(w)
					return
				}
				if r.Method == "POST" {
					close(held)
					<-release
					_, _ = io.WriteString(w, `{"id":"old-key"}`)
					return
				}
				w.WriteHeader(204)
			})
			runCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			close(done)
			m.run = &run{ctx: runCtx, cancel: cancel, done: done}
			gen := m.Status().Generation
			revision := m.Status().CredentialRevision
			// A failed assertion below must fail, not hang on the held provider response.
			t.Cleanup(unblock)
			type outcome struct {
				out dto.TestResult
				err error
			}
			result := make(chan outcome, 1)
			go func() { out, err := m.Test(t.Context()); result <- outcome{out, err} }()
			<-held
			switch change {
			case "same-mode":
				next := m.cred
				next.ClientSecret = strings.Repeat("replacement-", 3)
				m.Apply(t.Context(), true, next)
				require.Equal(t, gen, m.Status().Generation)
			case "reset":
				require.NoError(t, m.Reset(t.Context()))
			case "disable":
				m.Apply(t.Context(), false, m.cred)
			}
			require.NotEqual(t, revision, m.Status().CredentialRevision)
			unblock()
			got := <-result
			require.NoError(t, got.err)
			out := got.out
			require.Equal(t, revision, out.CredentialRevision)
			require.True(t, out.Stale)
			require.Equal(t, "pass", out.Checks[2].State)
			for _, row := range out.Checks[3:] {
				require.Equal(t, "skipped", row.State)
			}
		})
	}
	a, b := NewManager(ManagerConfig{}), NewManager(ManagerConfig{})
	require.Equal(t, a.Status().Generation, b.Status().Generation)
	require.NotEqual(t, a.Status().CredentialRevision, b.Status().CredentialRevision)
}

func TestSetupMalformedAndSecretResponses(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	secret, key, token := strings.Repeat("opaque-example-", 3), "tskey-"+"auth-"+"example-value", setupTokenValue()
	for index, body := range []string{`{"key":"` + key + `"}`, `{"id":`, strings.Repeat(" ", maxProviderBytes) + `{"id":"large"}`, secret + key + token} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					setupToken(w)
					return
				}
				_, _ = io.WriteString(w, body)
			})
			m.cred.ClientSecret = secret
			status, data := setupHTTP(t, setupRouter(m, true, false), t.Context())
			require.Equal(t, 200, status, string(data))
			out := setupResult(t, data)
			if strings.HasPrefix(body, `{"key"`) {
				require.Equal(t, "pass", out.Checks[1].State)
				require.Equal(t, dto.CodeTestCleanupFailed, out.Checks[2].Code)
			} else {
				require.Equal(t, dto.CodeProviderUnavailable, out.Checks[1].Code)
				require.Equal(t, "unknown", out.Checks[2].State)
			}
			for _, s := range []string{"tskey-", secret, key, token} {
				require.NotContains(t, string(data)+logs.String(), s)
			}
		})
	}
	// Provider error bodies are discarded rather than surfaced as diagnostic text.
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			setupToken(w)
			return
		}
		w.WriteHeader(403)
		_, _ = io.WriteString(w, secret+key+token)
	})
	_, data := setupHTTP(t, setupRouter(m, true, false), t.Context())
	for _, s := range []string{"tskey-", secret, token} {
		require.NotContains(t, string(data)+logs.String(), s)
	}
}

func TestSetupManualAndHubChecks(t *testing.T) {
	var calls atomic.Int32
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	m.cred.Mode = dto.ModeManual
	out, err := m.Test(t.Context())
	require.NoError(t, err)
	require.Zero(t, calls.Load())
	for _, row := range out.Checks[:3] {
		require.Equal(t, "skipped", row.State)
	}
	for _, row := range out.Checks[3:] {
		require.Equal(t, "pass", row.State)
	}
	// A joined hub without its tag or address fails that row only; no row claims the hub is disconnected.
	untagged := views.SliceOf([]string{"tag:other"})
	m.status.Self = &ipnstate.PeerStatus{Tags: &untagged}
	out, err = m.Test(t.Context())
	require.NoError(t, err)
	require.Equal(t, dto.TestCheck{ID: "hub-joined", State: "pass"}, out.Checks[3])
	require.Equal(t, dto.TestCheck{ID: "hub-tag", State: "fail"}, out.Checks[4])
	m.status.Self = nil
	out, err = m.Test(t.Context())
	require.NoError(t, err)
	require.Equal(t, "skipped", out.Checks[4].State)
	m.status.TailscaleIPs = nil
	out, err = m.Test(t.Context())
	require.NoError(t, err)
	require.Equal(t, dto.TestCheck{ID: "hub-address", State: "fail"}, out.Checks[5])
	m.conn.State = "joining"
	out, err = m.Test(t.Context())
	require.NoError(t, err)
	require.Equal(t, "fail", out.Checks[3].State)
	require.Equal(t, "skipped", out.Checks[4].State)
	require.Equal(t, "skipped", out.Checks[5].State)
}

func TestSetupAdminAndCSRFRefusal(t *testing.T) {
	var calls atomic.Int32
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, scenario := range []struct{ admin, csrf bool }{{false, false}, {true, true}} {
		status, _ := setupHTTP(t, setupRouter(m, scenario.admin, scenario.csrf, false), t.Context())
		require.Equal(t, 403, status)
	}
	require.Zero(t, calls.Load())
}

func setupTokenValue() string { return strings.Join([]string{"example", "bearer", "value"}, "-") }
