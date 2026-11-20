// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package volumehelper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveClassifiesPullAndDaemonFailures(t *testing.T) {
	for _, tt := range []struct {
		name          string
		inspect, pull int
		stream        string
		absent        bool
	}{
		{"daemon-inspect", 500, 200, "", false},
		{"not-found", 404, 404, "", true},
		{"denied", 404, 403, "", true},
		{"daemon-pull", 404, 500, "", false},
		{"registry-dial-response", 404, 500, `{"message":"Get \"https://registry.example.test/v2/\": dial tcp: lookup registry.example.test: no such host"}`, true},
		{"registry-timeout-response", 404, 500, `{"message":"Get \"https://registry.example.test/v2/\": dial tcp: i/o timeout"}`, false},
		{"registry-denied-response", 404, 500, `{"message":"pull access denied for registry.example.test/tools"}`, true},
		{"stream-denied", 404, 200, `{"error":"pull access denied"}`, true},
		{"stream-offline", 404, 200, `{"error":"dial tcp: connection refused"}`, true},
		{"stream-timeout", 404, 200, `{"error":"dial tcp: i/o timeout"}`, false},
		{"stream-daemon", 404, 200, `{"error":"synthetic daemon failure"}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/images/create") {
					w.WriteHeader(tt.pull)
					fmt.Fprint(w, tt.stream)
					return
				}
				w.WriteHeader(tt.inspect)
			}))
			t.Cleanup(server.Close)
			_, err := Resolve(t.Context(), newTestDockerClientInternal(t, server), "registry.example.test/tools:0.2.0")
			require.Error(t, err)
			var absent *ImageUnavailable
			require.Equal(t, tt.absent, errors.As(err, &absent))
			if !tt.absent {
				var transient *DockerError
				require.ErrorAs(t, err, &transient)
			}
		})
	}
}
func TestPullFailureKeepsDaemonTransportErrorsTransient(t *testing.T) {
	for _, err := range []error{&net.DNSError{Err: "synthetic DNS error", Name: "daemon.example.test"}, &net.OpError{Op: "dial", Err: errors.New("connection refused")}, context.DeadlineExceeded} {
		var transient *DockerError
		require.ErrorAs(t, pullFailure(err), &transient)
	}
}
