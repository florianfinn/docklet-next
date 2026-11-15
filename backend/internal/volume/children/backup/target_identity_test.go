// Source: docklet-next/issues/387
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original identity admission and delivery recovery transport proofs.

package backup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func identityDraft(kind, host string) dto.TargetDraftTest {
	location := dto.TargetLocation{Kind: kind}
	switch kind {
	case "local", "mount":
		location.HostID, location.Path = new(host), "/srv/archive"
	case "smb":
		location.Server, location.Share = "nas.example.com", "archive"
	case "nfs":
		location.Server, location.Share = "nas.example.com", "/archive"
	}
	return dto.TargetDraftTest{Target: dto.TargetInput{TargetLocation: location, Name: "Draft"}, ProbeHostID: host}
}

func TestDraftIdentityUnavailableAndDeliveryRecovery(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, identity := range []string{"absent", "empty"} {
			t.Run(mode+"/"+identity, func(t *testing.T) {
				f := newRemoteFenceHost(t, mode)
				if identity == "empty" {
					require.NoError(t, f.agentDB.service.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('hubHostId','')").Error)
				}
				var dockerCalls atomic.Int32
				dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					dockerCalls.Add(1)
					f.agentDB.serve(w, r)
				}))
				defer dockerServer.Close()
				cli, err := client.New(client.WithHost(dockerServer.URL), client.WithAPIVersion("1.41"))
				require.NoError(t, err)
				defer cli.Close()
				f.agentDB.service.deps.Docker = f.agentDB.service.deps.Docker.WithClient(cli)
				h := TargetHandler(f.hub, f.environments, &config.Config{}, t.Context())
				for _, kind := range []string{"local", "mount", "smb", "nfs"} {
					_, err := h.DraftTest(t.Context(), identityDraft(kind, f.id))
					var problem *dto.Problem
					require.ErrorAs(t, err, &problem, "hub must reconstruct the agent identity refusal")
					require.Equal(t, 503, problem.Status)
					require.Equal(t, "backup.target.identity-unavailable", problem.Code)
					require.Zero(t, dockerCalls.Load(), "identity refusal must precede every Docker request")
				}
				// Use the hub's real full-set producer and delivery receiver, including an empty set.
				set, err := f.hub.targetStore.SetForHost(t.Context(), f.id)
				require.NoError(t, err)
				require.Empty(t, set.Targets)
				payload, err := json.Marshal(set)
				require.NoError(t, err)
				var receipt base.ApiResponse[dto.TargetSetReceipt]
				require.NoError(t, f.environments.ProxyJSONRequest(t.Context(), f.id, http.MethodPut, "/api/environments/0/volumes/backups/targets", payload, &receipt))
				require.True(t, receipt.Success)
				for _, kind := range []string{"local", "mount", "smb", "nfs"} {
					result, err := h.DraftTest(t.Context(), identityDraft(kind, f.id))
					require.NoError(t, err, "delivery must repair the same probe without restarting the service")
					require.True(t, result.OK)
				}
				require.Positive(t, dockerCalls.Load())
				wrongOwner := identityDraft("local", f.id)
				wrongOwner.Target.HostID = new("other")
				before := dockerCalls.Load()
				_, err = backuptarget.RemoteJSON[base.ApiResponse[dto.TargetProbeResult]](t.Context(), f.environments.ProxyJSONRequest, f.id, http.MethodPost, "/api/environments/0/volumes/backups/targets/probe", dto.TargetProbeRequest{Target: wrongOwner.Target, ProbeHostID: f.id})
				require.EqualError(t, err, "backup.target.probe-host-mismatch")
				require.Equal(t, before, dockerCalls.Load())
			})
		}
	}
}

func TestDraftHubMissingIdentityStillProbesHostZero(t *testing.T) {
	for _, cfg := range []*config.Config{nil, {}} {
		f := newRuntimePlan(t)
		h := TargetHandler(f.service, nil, cfg, t.Context())
		for _, kind := range []string{"local", "mount", "smb", "nfs"} {
			request := identityDraft(kind, "0")
			result, err := h.Probe(t.Context(), dto.TargetProbeRequest{Target: request.Target, ProbeHostID: "0"})
			require.NoError(t, err, "the hub owns host zero before any target delivery")
			require.True(t, result.OK)
		}
	}
}
