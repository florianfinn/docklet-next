// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"encoding/json"
	"sync"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func TestEligibilityStatesAndCopies(t *testing.T) {
	host := Installation{Role: "hub", Version: "1.0.0", CurrentReference: "example.com/docklet-hub:1.0.0", Repository: "example.com/docklet-hub", Compose: true, Replayable: true}
	for _, name := range []string{"available", "current", "checking", "disabled", "feed unavailable", "busy", "not compose", "custom latest", "custom digest", "custom prerelease", "unsupported", "agent ceiling", "prior pin"} {
		t.Run(name, func(t *testing.T) {
			local := host
			feed := Feed{Candidate: &dto.SelfUpdateCandidate{Version: "1.0.1"}}
			hub := ""
			state, code := "available", ""
			switch name {
			case "current":
				feed.Candidate.Version = "1.0.0"
				state = "current"
			case "checking":
				feed.Checking = true
				state = "checking"
			case "disabled":
				feed.Code = "feature-disabled"
				state = "unavailable"
				code = feed.Code
			case "feed unavailable":
				feed.Code = "update.check-unavailable"
				state = "unavailable"
				code = feed.Code
			case "busy":
				local.Busy = true
				state = "not-eligible"
				code = "host.busy"
			case "not compose":
				local.Compose = false
				state = "not-eligible"
				code = CodeNotCompose
			case "custom latest":
				local.CurrentReference = "example.com/docklet-hub:latest"
				state = "not-eligible"
				code = CodeCustom
			case "custom digest":
				local.CurrentReference = "example.com/docklet-hub@sha256:" + digestHex
				state = "not-eligible"
				code = CodeCustom
			case "custom prerelease":
				local.Version = "1.0.0-rc.1"
				local.CurrentReference = "example.com/docklet-hub:1.0.0-rc.1"
				state, code = "not-eligible", CodeCustom
			case "unsupported":
				local.Replayable = false
				state = "not-eligible"
				code = CodeLayout
			case "agent ceiling":
				local.Role = "agent"
				hub = "1.0.0"
				state = "not-eligible"
				code = CodeHub
			case "prior pin":
				local.CurrentReference += "@sha256:" + digestHex
			}
			out := Eligibility(local, feed, hub)
			require.Equal(t, state, out.State)
			require.Equal(t, code, out.Code)
			require.NotEmpty(t, out.Manual)
			raw, err := json.Marshal(out)
			require.NoError(t, err)
			var wire dto.SelfUpdateEligibility
			require.NoError(t, json.Unmarshal(raw, &wire))
			require.Equal(t, out, wire)
		})
	}
}

const digestHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestEligibilityConcurrentSharedFeed(t *testing.T) {
	feed := Feed{Candidate: &dto.SelfUpdateCandidate{Version: "1.0.1", Reference: "example.com/hub:1.0.1@sha256:" + digestHex}}
	original := *feed.Candidate
	var workers sync.WaitGroup
	for i := 0; i < 30; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				role, repo := "agent", "example.com/agent"
				if i%3 == 0 {
					role, repo = "hub", "example.com/hub"
				}
				// Remaining workers model Direct and Edge agent views of the same feed.
				host := Installation{Role: role, Version: "1.0.0", CurrentReference: repo + ":1.0.0", Repository: repo, Compose: true, Replayable: true, Busy: i%5 == 0}
				out := Eligibility(host, feed, "1.0.1")
				if out.Candidate != nil {
					out.Candidate.Reference = "per-host"
				}
				out.Manual[0] = "per-host command"
			}
		}(i)
	}
	workers.Wait()
	require.Equal(t, original, *feed.Candidate)
}
func TestFrozenEnvelopeValidation(t *testing.T) {
	valid := dto.SelfUpdateEnvelope{OpID: "operation-1", TargetVersion: "1.0.1", TargetReference: "example.com/agent:1.0.1@sha256:" + digestHex, HubVersion: "1.0.1"}
	for _, name := range []string{"valid", "above hub", "own version ceiling differs", "wrong repo", "wrong tag", "no digest", "invalid op", "prerelease", "same version"} {
		t.Run(name, func(t *testing.T) {
			request := valid
			own := "1.0.0"
			want := false
			switch name {
			case "above hub":
				request.HubVersion = "1.0.0"
				want = true
			case "own version ceiling differs":
				own = "0.99.0"
			case "wrong repo":
				request.TargetReference = "example.com/hub:1.0.1@sha256:" + digestHex
				want = true
			case "wrong tag":
				request.TargetReference = "example.com/agent:1.0.0@sha256:" + digestHex
				want = true
			case "no digest":
				request.TargetReference = "example.com/agent:1.0.1"
				want = true
			case "invalid op":
				request.OpID = "../foreign"
				want = true
			case "prerelease":
				request.TargetVersion = "1.0.1-rc.1"
				want = true
			case "same version":
				own = "1.0.1"
				want = true
			}
			err := ValidateEnvelope(request, own, "example.com/agent")
			if want {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
