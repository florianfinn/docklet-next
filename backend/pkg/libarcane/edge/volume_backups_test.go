// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original actual-tunnel cancellation and response assembly bounds for volume backups.

package edge

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
	"uuid"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestVolumeBackupActualTunnelHubCancellationReachesHeldPreparation(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "request-cancel", true: "hub-disconnect"}[disconnect], func(t *testing.T) {
			entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			f := runtimeEdgeActual(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-r.Context().Done()
				close(cancelled)
				<-release
				w.WriteHeader(503)
			}))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			id := uuid.New().String()
			done := make(chan error, 1)
			go func() {
				_, err := NewCommandClient().Execute(ctx, f.tunnel, &CommandRequest{ID: id, Method: "GET", Path: "/api/environments/0/volumes/backups/example/download"})
				done <- err
			}()
			runtimeEdgeAwait(t, entered)
			entry := f.entry(id)
			require.NotNil(t, entry)
			if disconnect {
				f.tunnel.CloseWithReason("synthetic hub disconnect")
			} else {
				cancel()
				select {
				case err := <-done:
					require.Error(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("hub cancellation did not settle")
				}
				f.barrier(t)
				require.ErrorIs(t, entry.ctx.Err(), context.Canceled, "agent cancellation is observable after the message-loop barrier")
			}
			runtimeEdgeAwait(t, cancelled)
			require.Same(t, entry, f.entry(id), "registry retains the handler until its cleanup joins")
			once.Do(func() { close(release) })
			runtimeEdgeAwait(t, entry.done)
			if disconnect {
				select {
				case err := <-done:
					require.Error(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("hub request did not settle")
				}
			}
			require.Nil(t, f.entry(id))
		})
	}
}
func TestVolumeBackupCancellationRegistrationExcludesOrdinaryAndAsyncCommands(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/environments/0/volumes/data"},
		{"POST", "/api/environments/0/volumes/data/backups"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			f := runtimeEdgeActual(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-release
				w.WriteHeader(200)
				close(done)
			}))
			id := uuid.New().String()
			command, ok := ResolveEdgeCommandName(tc.method, tc.path, false).Get()
			require.True(t, ok)
			require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: id, Type: MessageTypeCommandRequest, Command: command, Method: tc.method, Path: tc.path}))
			runtimeEdgeAwait(t, entered)
			f.barrier(t)
			require.Nil(t, f.entry(id), "only synchronous backup preparation is registered")
			require.False(t, volumeBackupCancelableRequest(tc.method, tc.path))
			close(release)
			runtimeEdgeAwait(t, done)
		})
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/environments/0/volumes/backups/example/download"},
		{"GET", "/api/environments/0/volumes/backups/example/files/browse"},
		{"GET", "/api/environments/0/volumes/backups/example/files"},
		{"GET", "/api/environments/0/volumes/backups/example/has-path"},
		{"DELETE", "/api/environments/0/volumes/backups/example"},
	} {
		require.True(t, volumeBackupCancelableRequest(tc.method, tc.path), tc.path)
	}
}
func TestVolumeBackupActualTunnelResponseAssemblyBound(t *testing.T) {
	for _, size := range []int{dto.MaxVolumeResponseBytes, dto.MaxVolumeResponseBytes + 1} {
		t.Run(map[bool]string{false: "exact", true: "over"}[size > dto.MaxVolumeResponseBytes], func(t *testing.T) {
			server, tunnel := setupMockAgentServer(t, func(msg *TunnelMessage) *TunnelMessage {
				return &TunnelMessage{ID: msg.ID, Type: MessageTypeResponse, Status: 200, Body: bytes.Repeat([]byte(" "), size)}
			})
			defer server.Close()
			defer tunnel.CloseWithReason("")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			result, err := NewCommandClient().Execute(ctx, tunnel, &CommandRequest{Method: "GET", Path: "/api/environments/0/volumes/backups/overview"})
			if size > dto.MaxVolumeResponseBytes {
				var problem *dto.Problem
				require.ErrorAs(t, err, &problem)
				require.Equal(t, "volume.backup.listing-too-large", problem.Code)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Len(t, result.Body, size)
			}
		})
	}
}
