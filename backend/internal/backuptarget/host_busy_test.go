// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Validate only the narrow remote host.busy/409 admission envelope.

package backuptarget

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	host "github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRemoteHostBusyRequiresMatchingStatusAndCode(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"status":409,"code":"host.busy"}`, 409, true},
		{"wrong HTTP status", `{"status":409,"code":"host.busy"}`, 503, false},
		{"wrong body status", `{"status":503,"code":"host.busy"}`, 409, false},
		{"wrong code", `{"status":409,"code":"host.restarting"}`, 409, false},
		{"malformed", `{`, 409, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := func(context.Context, string, string, string, []byte, any) error {
				return &remenv.StatusError{StatusCode: tc.status, Body: []byte(tc.body)}
			}
			_, err := RemoteJSON[any](t.Context(), proxy, "host", "POST", "/api/environments/0/volumes/backups/targets/local/test", nil)
			require.Error(t, err)
			var problem *host.HostProblem
			require.Equal(t, tc.valid, errors.As(err, &problem))
		})
	}
}
