// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Scoped update capability and restore-point projection implementation for #229.

package edge

import (
	"bytes"
	"context"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestPointListResponseAssemblyBound(t *testing.T) {
	for _, size := range []int{dto.MaxRestorePointResponseBytes, dto.MaxRestorePointResponseBytes + 1} {
		t.Run(map[bool]string{true: "over", false: "exact"}[size > dto.MaxRestorePointResponseBytes], func(t *testing.T) {
			server, tunnel := setupMockAgentServer(t, func(msg *TunnelMessage) *TunnelMessage {
				return &TunnelMessage{ID: msg.ID, Type: MessageTypeResponse, Status: 200, Body: bytes.Repeat([]byte(" "), size)}
			})
			defer server.Close()
			defer tunnel.CloseWithReason("")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, _, body, err := ProxyRequest(ctx, tunnel, http.MethodGet, "/api/environments/0/updater/restore-points", "", nil, nil)
			if size > dto.MaxRestorePointResponseBytes {
				require.Error(t, err)
				require.Nil(t, body)
			} else {
				require.NoError(t, err)
				require.Len(t, body, size)
			}
		})
	}
}
