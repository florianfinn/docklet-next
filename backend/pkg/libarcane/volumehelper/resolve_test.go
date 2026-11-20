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

	"github.com/moby/moby/client"
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
		{"registry-timeout-response", 404, 500, `{"message":"Get \"https://registry.example.test/v2/\": dial tcp: i/o timeout"}`, true},
		{"registry-denied-response", 404, 500, `{"message":"pull access denied for registry.example.test/tools"}`, true},
		{"stream-denied", 404, 200, `{"error":"pull access denied"}`, true},
		{"stream-offline", 404, 200, `{"error":"dial tcp: connection refused"}`, true},
		{"stream-timeout", 404, 200, `{"error":"dial tcp: i/o timeout"}`, true},
		{"stream-deadline", 404, 200, `{"error":"context deadline exceeded"}`, false},
		{"registry-deadline-response", 404, 500, `{"message":"Get \"https://registry.example.test/v2/\": net/http: request canceled (Client.Timeout exceeded)"}`, false},
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
			_, err := Resolve(t.Context(), newTestDockerClientInternal(t, server), "registry.example.test/tools:0.2.0", nil)
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

// D1: an authenticated pull's daemon error may reflect the credential; neither
// the message nor the unwrap chain keeps it, for either result class.
func TestResolvePullErrorsAreNeutral(t *testing.T) {
	const sentinel = "synthetic-registry-credential-sentinel"
	for _, tt := range []struct {
		name   string
		status int
		stream bool
		absent bool
	}{
		{"http-daemon", 500, false, false},
		{"http-denied", 403, false, true},
		{"stream-daemon", 200, true, false},
		{"stream-denied", 200, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if !strings.Contains(r.URL.Path, "/images/create") {
					w.WriteHeader(404)
					return
				}
				reflected := "registry reflected " + r.Header.Get("X-Registry-Auth")
				if tt.absent {
					reflected = "pull access denied: " + reflected
				}
				w.WriteHeader(tt.status)
				if tt.stream {
					fmt.Fprintf(w, `{"error":%q}`+"\n", reflected)
				} else {
					fmt.Fprintf(w, `{"message":%q}`, reflected)
				}
			}))
			t.Cleanup(server.Close)
			_, err := Resolve(t.Context(), newTestDockerClientInternal(t, server), "registry.example.test/tools:0.2.0", func(context.Context, string) (client.ImagePullOptions, error) {
				return client.ImagePullOptions{RegistryAuth: sentinel}, nil
			})
			require.Error(t, err)
			var absent *ImageUnavailable
			require.Equal(t, tt.absent, errors.As(err, &absent))
			for e := err; e != nil; e = errors.Unwrap(e) {
				require.NotContains(t, e.Error(), sentinel)
			}
		})
	}
}
