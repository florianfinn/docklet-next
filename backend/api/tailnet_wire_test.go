// Source: docklet-next/issues/586
// Revision: 30f576f655ca000140c41896ec33444d8e418a20
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original wire proof for optional tailnet provider statuses through the hub JSON format (M37 T1).

package api

import (
	"bytes"
	"strings"
	"testing"

	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// The hub encodes with encoding/json/v2, where omitempty never drops a zero number; an absent provider status must
// stay absent on the wire, because the web decoder accepts only HTTP statuses.
func TestTailnetProviderStatusIsOmittedWhenAbsent(t *testing.T) {
	values := map[string]any{
		"test check":   tailnettypes.TestCheck{ID: "agent-key", State: "pass"},
		"provisioning": tailnettypes.Provisioning{State: "ok"},
		"problem":      &tailnettypes.Problem{Code: tailnettypes.CodeTestRunning, Status: 409},
	}
	for name, value := range values {
		var buf bytes.Buffer
		if err := jsonV2Format.Marshal(&buf, value); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(buf.String(), "providerStatus") {
			t.Errorf("%s: providerStatus sent without a provider answer: %s", name, buf.String())
		}
	}
	var buf bytes.Buffer
	if err := jsonV2Format.Marshal(&buf, tailnettypes.TestCheck{ID: "agent-key", State: "fail", Code: tailnettypes.CodeMintRefused, ProviderStatus: 403}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"providerStatus":403`) {
		t.Errorf("provider status lost: %s", buf.String())
	}
}
