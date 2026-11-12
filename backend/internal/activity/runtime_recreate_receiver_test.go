// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original receiver-side strict edit and semantic size guards independent of hub intake.

package activity

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	containerDTO "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func TestRecreateReceiverRejectsUnknownNestedAndOversizeEdit(t *testing.T) {
	for _, mode := range []string{"unknown", "nested-unknown", "nested-type", "oversize", "wire-whitespace"} {
		t.Run(mode, func(t *testing.T) {
			f, probe := receiverProbeFixture(t)
			defer probe.unblock()
			command := f.command()
			command.Operation = RuntimeRecreateContainer
			command.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}
			command.Options = RuntimeOptions{Recreate: &containerDTO.Edit{}}
			if mode == "nested-unknown" {
				command.Options.Recreate.HostConfig = &containerDTO.HostConfigEdit{}
			}
			envelope := receiverEnvelope(t, command)
			if mode == "oversize" {
				edit := &containerDTO.Edit{WorkingDir: new(strings.Repeat("x", 64<<10))}
				envelope.Options.Recreate = edit
				// This claimed digest deliberately matches the oversized edit. Removing the
				// receiver size guard must reach acceptance, rather than another digest veto.
				envelope.Identity.RequestDigest = runtimeDigestInternal([5]any{string(command.Operation), command.Target.Kind, command.Target.ID, runtimeRecreateSemantic(*edit), command.Target.ParentProjectID})
			}
			body, err := json.Marshal(envelope, json.FormatNilSliceAsNull(true))
			require.NoError(t, err)
			switch mode {
			case "unknown":
				body = []byte(strings.Replace(string(body), `"recreate":{`, `"recreate":{"unknown":true,`, 1))
			case "nested-unknown":
				body = []byte(strings.Replace(string(body), `"hostConfig":{`, `"hostConfig":{"unknown":true,`, 1))
			case "wire-whitespace":
				body = []byte(strings.Replace(string(body), `"recreate":{`, `"recreate":{`+strings.Repeat(" ", 64<<10), 1))
			case "nested-type":
				body = []byte(strings.Replace(string(body), `"recreate":{`, `"recreate":{"command":[1],`, 1))
			}
			server := receiverHTTPServer(t, f, time.Minute)
			status, response := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, body)
			require.Equal(t, 400, status, string(response))
			receiverCounts(t, f, 0, 0, 0)
			require.Zero(t, probe.calls.Load())
		})
	}
}
