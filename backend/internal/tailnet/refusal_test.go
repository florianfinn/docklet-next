// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original real-shaped provider and wrapped join refusal regression proofs.

package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/tailnet"
	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale"
)

func TestTagRefusalProviderToResponse(t *testing.T) {
	for _, tag := range []string{HubTag, AgentTag} {
		t.Run(tag, func(t *testing.T) {
			deletes := 0
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					setupToken(w)
					return
				}
				if r.Method == "DELETE" {
					deletes++
				}
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "requested tags [" + tag + "] are invalid or not permitted"})
			})
			status, body := setupHTTP(t, setupRouter(m, true, false), t.Context())
			require.Equal(t, 200, status)
			result := setupResult(t, body)
			require.Equal(t, dto.TestCheck{ID: "agent-key", State: "fail", Code: dto.CodeTagsNotPermitted, ProviderStatus: 400, RefusedTags: []string{tag}}, result.Checks[1])
			require.Equal(t, "skipped", result.Checks[2].State)
			require.Zero(t, deletes)
			_, _, err := m.ProvisionHostKey(t.Context(), "")
			var problem *dto.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, dto.CodeTagsNotPermitted, problem.Code)
			require.Equal(t, []string{tag}, problem.RefusedTags)
			require.Equal(t, "rejected", m.Status().Provisioning.State)
			require.Equal(t, []string{tag}, m.Status().Provisioning.RefusedTags)
			require.ErrorAs(t, m.RouteRefusal(), &problem)
			require.Equal(t, dto.CodeTagsNotPermitted, problem.Code)
			require.Equal(t, []string{tag}, problem.RefusedTags)
		})
	}
}

func TestTagRefusalFilteringAndBounds(t *testing.T) {
	secret := strings.Join([]string{"tskey", "client", "example-secret"}, "-")
	for _, tc := range []struct {
		name, message string
		tags          []string
		code          bool
	}{
		{"filter", "requested tags [tag:docklet-hub " + secret + " tag:INVALID tag:bad_value tag:docklet-agent] are invalid or not permitted", []string{HubTag, AgentTag}, true},
		{"cap", "requested tags [tag:a tag:b tag:c tag:d tag:e tag:f tag:g tag:h tag:i] are invalid or not permitted", []string{"tag:a", "tag:b", "tag:c", "tag:d", "tag:e", "tag:f", "tag:g", "tag:h"}, true},
		{"oversized", strings.Repeat(" ", maxProviderBytes) + "requested tags [tag:docklet-hub] are invalid or not permitted", nil, false},
		{"other", "not requested tags [" + secret + "] are invalid or not permitted", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					setupToken(w)
					return
				}
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": tc.message})
			})
			for _, mint := range []func() error{
				func() error { _, e := m.cfg.Provisioner.MintTestKey(t.Context(), "example-token"); return e },
				func() error {
					_, e := m.cfg.Provisioner.MintHostKey(t.Context(), "example-client", "example-secret")
					return e
				},
			} {
				var p *dto.Problem
				require.ErrorAs(t, mint(), &p)
				require.Equal(t, tc.tags, p.RefusedTags)
				require.Equal(t, tc.code, p.Code == dto.CodeTagsNotPermitted)
				require.NotContains(t, p.Error(), secret)
			}
		})
	}
}

func TestJoinTagRefusalClassification(t *testing.T) {
	message := "requested tags [tag:docklet-hub] are invalid or not permitted"
	for _, err := range []error{
		tailscale.ErrResponse{Status: 400, Message: message},
		fmt.Errorf("wrapped: %w", tailscale.ErrResponse{Status: 400, Message: message}),
		&tailscale.ErrResponse{Status: 400, Message: message},
		errors.New("tsnet.Up: backend: " + message),
		errors.New(`tsnet.Up: resolving auth key: Status: 400, Message: "` + message + `"`),
		fmt.Errorf("wrapper: %w", errors.New(`backend: "`+message+`"`)),
	} {
		p := joinRefusal(err)
		require.NotNil(t, p)
		require.Equal(t, dto.CodeTagsNotPermitted, p.Code)
		require.Equal(t, []string{HubTag}, p.RefusedTags)
	}
	require.Equal(t, dto.CodeCredentialRejected, joinRefusal(errors.New("tsnet.Up: backend: invalid key")).Code)
	require.Equal(t, dto.CodeCredentialRejected, joinRefusal(tailscale.ErrResponse{Status: 403, Message: message}).Code)
	require.Nil(t, joinRefusal(errors.New(strings.Repeat("x", maxProviderBytes)+message)))
	require.Nil(t, joinRefusal(context.DeadlineExceeded))
}

func TestJoinRefusalConnectionProblemAndTestResponse(t *testing.T) {
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"access_token":"example"}`) })
	m.cred.Mode = dto.ModeManual // isolate hub check from provider admission
	m.cred.AuthKey = strings.Join([]string{"tskey", "auth", "example"}, "-")
	t.Cleanup(func() { m.Close(context.Background()) })
	m.conn = dto.Connection{State: "error", Code: dto.CodeTagsNotPermitted, RefusedTags: []string{HubTag}}
	m.cfg.MTLSDisabled = true
	m.enabled = false // retry must not run while off
	p := m.routeProblemLocked()
	require.Equal(t, dto.CodeNotConnected, p.Code)
	m.enabled = true
	p = m.routeProblemLocked()
	require.Equal(t, dto.CodeTagsNotPermitted, p.Code)
	require.Equal(t, []string{HubTag}, p.RefusedTags)
	// The retried join still projects the terminal provider refusal.
	m.cfg.Factory = func(NodeConfig) (Node, error) {
		return nil, errors.New("backend: requested tags [tag:docklet-hub] are invalid or not permitted")
	}
	status, body := setupHTTP(t, setupRouter(m, true, false), t.Context())
	require.Equal(t, 200, status)
	out := setupResult(t, body)
	require.Equal(t, dto.CodeTagsNotPermitted, out.Checks[3].Code)
	require.Equal(t, []string{HubTag}, out.Checks[3].RefusedTags)
	require.Equal(t, dto.CodeTagsNotPermitted, m.Status().Connection.Code)
	require.Equal(t, []string{HubTag}, m.Status().Connection.RefusedTags)
	require.NotContains(t, string(body), "requested tags")
	require.NotContains(t, string(body), `"providerStatus":0`)
	m.enabled = false
	m.conn = dto.Connection{State: "off"}
	_, body = setupHTTP(t, setupRouter(m, true, false), t.Context())
	require.Equal(t, dto.CodeNotConnected, setupResult(t, body).Checks[3].Code)
}

func TestRetryJoinUnsubscribesAfterImmediateRefusal(t *testing.T) {
	m := NewManager(ManagerConfig{MTLSDisabled: true, StateDir: t.TempDir(), Factory: func(NodeConfig) (Node, error) {
		return nil, errors.New("backend: requested tags [tag:docklet-hub] are invalid or not permitted")
	}})
	t.Cleanup(func() { m.Close(context.Background()) })
	m.enabled = true
	m.cred = Credential{Mode: dto.ModeManual, AuthKey: "example-key"}
	m.conn = dto.Connection{State: "error", Code: dto.CodeCredentialRejected}
	revision := m.Status().CredentialRevision
	require.True(t, m.RetryJoin(t.Context(), revision))
	m.mu.Lock()
	require.Empty(t, m.listeners)
	m.mu.Unlock()
	require.Equal(t, dto.CodeTagsNotPermitted, m.Connection().Code)
	require.Equal(t, revision, m.Status().CredentialRevision)
}
