// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host edit and fencing regression tests.

package environment

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestHostNameRegistrationAndConcurrentRename(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	for _, name := range []string{"", "  ", strings.Repeat("a", 41), " eXaMpLe "} {
		_, err := f.service.CreateEnvironment(t.Context(), &Environment{Name: name, ApiUrl: "http://agent.example.com"}, nil, nil)
		require.Error(t, err, name)
		var problem *dto.HostProblem
		require.ErrorAs(t, err, &problem)
		require.Contains(t, []string{"host.name-invalid", "host.name-taken"}, problem.Code)
	}
	hosts := make([]*Environment, 2)
	for i, name := range []string{"First", "Second"} {
		host, err := f.service.CreateEnvironment(t.Context(), &Environment{Name: name, ApiUrl: "http://agent.example.com"}, nil, nil)
		require.NoError(t, err)
		hosts[i] = host
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for i, host := range hosts {
		wg.Go(func() {
			<-start
			_, err := f.service.UpdateEnvironment(t.Context(), host.ID, map[string]any{"name": []string{" Same ", "sAME"}[i]}, nil, nil)
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			var p *dto.HostProblem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "host.name-taken", p.Code)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	_, err := f.service.UpdateEnvironment(t.Context(), hosts[0].ID, map[string]any{"name": strings.Repeat("ä", 41)}, nil, nil)
	require.Error(t, err)
}

func TestHostColorHandlerAbsentNullAndInvalid(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	h := &EnvironmentHandler{environmentService: f.service}
	defer h.background.Wait()
	for _, tc := range []struct {
		body  string
		valid bool
		color *string
	}{
		{`{"color":"#12abcd"}`, true, new("#12abcd")},
		{`{"color":"#12ABCD"}`, false, nil}, {`{"color":"red"}`, false, nil},
		{`{}`, true, new("#12abcd")}, {`{"color":null}`, true, nil},
	} {
		var body dto.Update
		require.NoError(t, json.Unmarshal([]byte(tc.body), &body))
		_, err := h.UpdateEnvironment(t.Context(), &UpdateEnvironmentInput{ID: f.id, Body: body})
		if tc.valid {
			require.NoError(t, err)
			row, e := f.service.GetEnvironmentByID(t.Context(), f.id)
			require.NoError(t, e)
			require.Equal(t, tc.color, row.Color)
		} else {
			require.Error(t, err)
		}
	}
}

func TestCredentialRevisionAllWritersAndRollback(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
	f.service.apiKeys = keys
	read := func() *Environment {
		row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
		require.NoError(t, err)
		return row
	}
	revision := read().CredentialRevision
	for _, updates := range []map[string]any{
		{"access_token": "test-only"},
		{"api_url": "https://agent.example.com", "access_token": "test-only"},
		{"api_key_id": "example"},
	} {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, updates, nil, nil)
		require.NoError(t, err)
		revision++
		require.Equal(t, revision, read().CredentialRevision)
		if revision == 1 {
			require.Equal(t, new("example"), read().ApiKeyID)
			require.Equal(t, "http://agent.example.com", read().ApiUrl)
		}
	}
	before := read()
	_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{"name": "", "access_token": "example"}, nil, nil)
	require.Error(t, err)
	require.Equal(t, before.CredentialRevision, read().CredentialRevision)
	require.Equal(t, before.AccessToken, read().AccessToken)
	_, err = f.service.RegenerateEnvironmentApiKey(t.Context(), read(), "operator", "operator")
	require.NoError(t, err)
	revision++
	require.Equal(t, revision, read().CredentialRevision)
	require.NoError(t, f.service.EnsureLocalEnvironment(t.Context(), "http://hub.example.com"))
	require.NoError(t, f.service.EnsureLocalEnvironment(t.Context(), "https://hub.example.com"))
	local, err := f.service.GetEnvironmentByID(t.Context(), "0")
	require.NoError(t, err)
	require.EqualValues(t, 1, local.CredentialRevision)
}
