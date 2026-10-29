// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original tests for the shared agent access envelope.

package handlerutil

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

const syntheticPrivate = "synthetic-private.example.com"

func TestTranslateRemoteProxyErrorAgentEnvelope(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New(syntheticPrivate)}
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"update required", fmt.Errorf("%s: %w", syntheticPrivate, remenv.ErrAgentUpdateRequired), 409, "agent.update-required"},
		{"invalid token", fmt.Errorf("%s: %w", syntheticPrivate, remenv.ErrAgentInvalidToken), 502, "agent.invalid-token"},
		{"upstream 401", &remenv.StatusError{StatusCode: 401, Body: []byte(syntheticPrivate)}, 502, "agent.invalid-token"},
		{"upstream 403", &remenv.StatusError{StatusCode: 403, Body: []byte(syntheticPrivate)}, 502, "agent.invalid-token"},
		{"edge not connected", fmt.Errorf("%s: %w", syntheticPrivate, remenv.ErrEnvironmentUnavailable), 502, "agent.unavailable"},
		{"tunnel unavailable", &remenv.TransportError{Err: remenv.ErrEnvironmentUnavailable}, 502, "agent.unavailable"},
		{"dial failure", &remenv.TransportError{Err: dial}, 502, "agent.unavailable"},
		{"deadline", &remenv.TransportError{Err: context.DeadlineExceeded}, 502, "agent.unavailable"},
		{"stalled access check", fmt.Errorf("check: %w", context.DeadlineExceeded), 502, "agent.unavailable"},
		{"already mapped", fmt.Errorf("wrapped: %w", AgentAccess(remenv.ErrAgentUpdateRequired)), 409, "agent.update-required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := TranslateRemoteProxyError(tc.err)
			var access *AgentAccessError
			require.ErrorAs(t, err, &access)
			require.Equal(t, tc.status, access.GetStatus())
			body, mErr := json.Marshal(err)
			require.NoError(t, mErr)
			require.JSONEq(t, `{"success":false,"code":"`+tc.code+`"}`, string(body))
			require.NotContains(t, err.Error(), syntheticPrivate)
		})
	}
}

func TestTranslateRemoteProxyErrorHidesWrappedText(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"cancelled transport", &remenv.TransportError{Err: fmt.Errorf("%s: %w", syntheticPrivate, context.Canceled)}, 502},
		{"read failure", &remenv.TransportError{Err: errors.New(syntheticPrivate)}, 502},
		{"decode failure", &remenv.DecodeError{Err: errors.New(syntheticPrivate)}, 500},
		{"decode deadline", &remenv.DecodeError{Err: fmt.Errorf("%s: %w", syntheticPrivate, context.DeadlineExceeded)}, 500},
		{"unclassified", errors.New(syntheticPrivate), 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := TranslateRemoteProxyError(tc.err)
			var access *AgentAccessError
			require.False(t, errors.As(err, &access))
			var typed huma.StatusError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, tc.status, typed.GetStatus())
			body, mErr := json.Marshal(err)
			require.NoError(t, mErr)
			require.NotContains(t, string(body), syntheticPrivate)
			require.NotContains(t, err.Error(), syntheticPrivate)
		})
	}
}

func TestTranslateRemoteProxyErrorKeepsDomainStatus(t *testing.T) {
	for _, status := range []int{400, 404, 409, 422, 500} {
		err := TranslateRemoteProxyError(&remenv.StatusError{StatusCode: status, Body: []byte("domain detail")})
		var access *AgentAccessError
		require.False(t, errors.As(err, &access))
		var typed huma.StatusError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, status, typed.GetStatus())
		require.Contains(t, err.Error(), "domain detail")
	}
}

func TestAgentVerdictExcludesUnavailable(t *testing.T) {
	require.Nil(t, AgentVerdict(&remenv.TransportError{Err: remenv.ErrEnvironmentUnavailable}))
	require.Nil(t, AgentVerdict(remenv.ErrEnvironmentUnavailable))
	require.Nil(t, AgentVerdict(&remenv.StatusError{StatusCode: 404}))
	require.Equal(t, "agent.update-required", AgentVerdict(remenv.ErrAgentUpdateRequired).Code)
	require.Equal(t, "agent.invalid-token", AgentVerdict(&remenv.StatusError{StatusCode: 401}).Code)
}
