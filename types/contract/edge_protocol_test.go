// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Edge protocol marker tests.

package contract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEdgeProtocol(t *testing.T) {
	cases := []struct {
		name string
		caps []string
		want EdgeProtocol
	}{
		{"missing counts as one", []string{"chunked-request"}, EdgeProtocol{Version: 1}},
		{"own marker", []string{"chunked-request", "edge-protocol/1"}, EdgeProtocol{Version: 1, Advertised: true}},
		{"future marker is recorded", []string{"edge-protocol/2"}, EdgeProtocol{Version: 2, Advertised: true}},
		{"duplicate is malformed", []string{"edge-protocol/1", "edge-protocol/1"}, EdgeProtocol{Version: 1, Malformed: true}},
		{"conflict is malformed", []string{"edge-protocol/1", "edge-protocol/2"}, EdgeProtocol{Version: 1, Malformed: true}},
		{"non-numeric is malformed", []string{"edge-protocol/x"}, EdgeProtocol{Version: 1, Malformed: true}},
		{"leading zero is malformed", []string{"edge-protocol/01"}, EdgeProtocol{Version: 1, Malformed: true}},
		{"zero is malformed", []string{"edge-protocol/0"}, EdgeProtocol{Version: 1, Malformed: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ParseEdgeProtocol(tc.caps))
		})
	}
}

func TestWithoutEdgeProtocolKeepsOtherCapabilities(t *testing.T) {
	require.Equal(t, []string{"chunked-request", "proto-parity-v1"},
		WithoutEdgeProtocol([]string{"chunked-request", "edge-protocol/2", "proto-parity-v1", "edge-protocol/1"}))
	require.Equal(t, "edge-protocol/1", EdgeProtocolCapability())
}
