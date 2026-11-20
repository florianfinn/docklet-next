// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package image

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestS4SafeUncertaintyVocabularyAndCapacity(t *testing.T) {
	peers := []RuntimeIdentityPeer{{PeerID: "example-peer", Name: "Example peer", Reason: "compose.identity-unreadable", File: "nested/service.env"}}
	u := NewRuntimeUncertainty(peers)
	b := &RuntimeIdentityBlocker{Name: "Example peer", PeerID: "example-peer", Reason: "compose.name-undeterminable", Confirmable: true, Uncertainty: u}
	safe := SafeRuntimeIdentityBlocker("runtime.target-unavailable", b)
	require.True(t, safe.Confirmable)
	peers[0].Name = "changed"
	require.Equal(t, "Example peer", safe.Uncertainty.Peers[0].Name, "decoder returns owned confirmation data")
	for _, fault := range []string{"id", "reason", "file", "digest", "count", "peers", "flag", "outer-file"} {
		t.Run(fault, func(t *testing.T) {
			data, err := json.Marshal(b)
			require.NoError(t, err)
			var bad RuntimeIdentityBlocker
			require.NoError(t, json.Unmarshal(data, &bad))
			switch fault {
			case "id":
				bad.Uncertainty.Peers[0].PeerID = "../unsafe"
			case "reason":
				bad.Uncertainty.Peers[0].Reason = "raw.diagnostic"
			case "file":
				bad.Uncertainty.Peers[0].File = "../external.env"
			case "digest":
				bad.Uncertainty.Digest = "invalid"
			case "count":
				bad.Uncertainty.Count = 17
			case "peers":
				bad.Uncertainty.Peers = nil
			case "flag":
				bad.Confirmable = false
			case "outer-file":
				bad.File = "/external.env"
			}
			result := SafeRuntimeIdentityBlocker("runtime.target-unavailable", &bad)
			require.NotNil(t, result)
			require.False(t, result.Confirmable, "malformed confirmation data must fail closed")
		})
	}
	for _, file := range []string{"/absolute.env", "../outside.env", "nested/../outside.env", "C:/outside.env", `\\server\file.env`, "line\n.env", string([]byte{0xff})} {
		require.False(t, SafeRuntimeFile(file), file)
	}
	require.True(t, SafeRuntimeFile("nested/service.env"))
	require.True(t, ValidRuntimeUncertaintyDigest(strings.Repeat("A", 64)))
	require.False(t, ValidRuntimeUncertaintyDigest(strings.Repeat("z", 64)))
}

func TestS4UncertaintyDigestUsesFullSortedIdentitySet(t *testing.T) {
	a := RuntimeIdentityPeer{PeerID: "example-a", Name: "A", Reason: "compose.identity-unreadable"}
	b := RuntimeIdentityPeer{PeerID: "example-b", Name: "B", Reason: "compose.selection-unsupported"}
	first := NewRuntimeUncertainty([]RuntimeIdentityPeer{a, b})
	second := NewRuntimeUncertainty([]RuntimeIdentityPeer{b, a})
	require.Equal(t, first, second, "row order cannot alter confirmation or display")
	b.Reason = "compose.identity-unreadable"
	require.NotEqual(t, first.Digest, NewRuntimeUncertainty([]RuntimeIdentityPeer{a, b}).Digest, "reason changes invalidate an acknowledgement")
}
