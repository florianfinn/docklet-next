// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	"encoding/json"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDraftProbeTransientCleanupAndNoMetadata(t *testing.T) {
	for _, outcome := range []string{"ok", "write-failed", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			volumeName := ""
			helpers, volumes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/volumes/create"):
					var input client.VolumeCreateOptions
					require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
					volumeName = input.Name
					require.True(t, strings.HasPrefix(volumeName, "docklet-target-probe-"))
					_, _ = w.Write([]byte(`{"Name":"` + volumeName + `"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					_, _ = w.Write([]byte(`{"Id":"probe"}`))
				case r.Method == "DELETE":
					if strings.Contains(r.URL.Path, "/volumes/") {
						volumes++
					} else {
						helpers++
					}
					require.NoError(t, r.Context().Err())
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/start"):
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			target := dto.SyncedTarget{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Draft", Secret: "test-only"}
			result, err := Probe(ctx, cli, "tools:1.0.0", target, "0", func(ctx context.Context, _, _ string, args []string) (string, string, error) {
				require.NotContains(t, strings.Join(args, " "), target.Secret)
				if args[0] == "rm" {
					deadline, bounded := ctx.Deadline()
					require.True(t, bounded, "probe cleanup must have a deadline")
					require.Positive(t, time.Until(deadline))
					require.LessOrEqual(t, time.Until(deadline), 30*time.Second)
					require.NoError(t, ctx.Err())
					return "", "", nil
				}
				deadline, bounded := ctx.Deadline()
				require.True(t, bounded)
				require.LessOrEqual(t, time.Until(deadline), 45*time.Second, "reserve cleanup within the 60-second probe budget")
				if outcome == "cancel" {
					cancel()
					<-ctx.Done()
					return "", "", ctx.Err()
				}
				if outcome == "write-failed" {
					return "", "", errors.New("permission denied")
				}
				return "", "", nil
			})
			require.NoError(t, err)
			require.Equal(t, outcome == "ok", result.OK)
			require.NotEmpty(t, volumeName)
			require.Equal(t, 1, helpers)
			require.Equal(t, 1, volumes)
			wire, err := json.Marshal(result)
			require.NoError(t, err)
			require.NotContains(t, string(wire), target.Secret)
		})
	}
}
func TestDraftEditUsesStoredSecretAndRefusesMovedLocation(t *testing.T) {
	store := testStore(t)
	svc := NewService(Dependencies{Store: store})
	secret := "test-only"
	input := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive", Secret: &secret}
	target, err := svc.Create(t.Context(), input)
	require.NoError(t, err)
	input.Secret = nil
	draft, err := svc.Draft(t.Context(), dto.TargetDraftTest{Target: input, TargetID: target.ID, ProbeHostID: "host-a"})
	require.NoError(t, err)
	require.Equal(t, secret, *draft.Secret)
	input.Share = "other"
	_, err = svc.Draft(t.Context(), dto.TargetDraftTest{Target: input, TargetID: target.ID, ProbeHostID: "0"})
	require.EqualError(t, err, "backup.target.immutable")
	var count int64
	require.NoError(t, store.DB.Model(&Target{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, store.DB.Model(&Check{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestDraftLocalOwnerMustExecute(t *testing.T) {
	svc := NewService(Dependencies{})
	_, err := svc.Draft(t.Context(), dto.TargetDraftTest{Target: dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("host-a"), Path: "/srv/archive"}, Name: "Draft"}, ProbeHostID: "host-b"})
	require.EqualError(t, err, "backup.target.probe-host-mismatch")
}

type probeTransport func(*http.Request) (*http.Response, error)

func (f probeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProbeCleanupRetainsOverallDeadline(t *testing.T) {
	deadline := time.Now().Add(5 * time.Second)
	calls := 0
	transport := probeTransport(func(r *http.Request) (*http.Response, error) {
		actual, ok := r.Context().Deadline()
		require.True(t, ok)
		require.False(t, actual.After(deadline), "cleanup must not exceed the original probe budget")
		calls++
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	cli, err := client.New(client.WithHost("http://daemon.example.com"), client.WithAPIVersion("1.41"), client.WithHTTPClient(&http.Client{Transport: transport}))
	require.NoError(t, err)
	defer cli.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	session := &Session{ID: "probe", transient: "probe-volume", cli: cli, cleanupDeadline: deadline}
	require.NoError(t, session.Close(ctx))
	require.Equal(t, 2, calls, "helper and transient volume are removed after cancellation")
}
