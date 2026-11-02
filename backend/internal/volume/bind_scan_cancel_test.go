// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/backup"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBindScanCancellationRemovesHelper(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			entered := make(chan struct{})
			removed := make(chan struct{})
			var once sync.Once
			var mu sync.Mutex
			command := ""
			creates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				p := r.URL.Path
				switch {
				case strings.HasSuffix(p, "/info"):
					_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
				case strings.HasSuffix(p, "/containers/json"):
					_, _ = w.Write([]byte(`[{"Id":"app","Mounts":[{"Type":"bind","Source":"/srv/app.conf","Destination":"/config"}]}]`))
				case strings.Contains(p, "/images/"):
					_, _ = w.Write([]byte(`{"Id":"tools"}`))
				case strings.HasSuffix(p, "/containers/create"):
					mu.Lock()
					creates++
					id := "witness"
					if creates > 1 {
						id = "scan"
					}
					mu.Unlock()
					_, _ = w.Write([]byte(`{"Id":"` + id + `"}`))
				case r.Method == "DELETE":
					if strings.HasSuffix(p, "/scan") {
						once.Do(func() { close(removed) })
					}
					w.WriteHeader(204)
				case strings.HasSuffix(p, "/exec"):
					var in struct{ Cmd []string }
					require.NoError(t, json.NewDecoder(r.Body).Decode(&in))
					mu.Lock()
					command = in.Cmd[0]
					mu.Unlock()
					_, _ = w.Write([]byte(`{"Id":"exec"}`))
				case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/start"):
					connection, buffer, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					defer connection.Close()
					_, err = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
					require.NoError(t, err)
					require.NoError(t, buffer.Flush())
					mu.Lock()
					cmd := command
					mu.Unlock()
					if cmd == "sh" {
						frame := make([]byte, 8)
						frame[0] = 1
						binary.BigEndian.PutUint32(frame[4:], 4)
						_, _ = connection.Write(append(frame, []byte("file")...))
						return
					}
					close(entered)
					_, _ = io.Copy(io.Discard, connection)
				case strings.Contains(p, "/exec/") && strings.HasSuffix(p, "/json"):
					_, _ = w.Write([]byte(`{"Running":false,"ExitCode":0}`))
				case strings.HasSuffix(p, "/start"):
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected %s", p)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			s := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if deadline {
				ctx, cancel = context.WithTimeout(t.Context(), 150*time.Millisecond)
				defer cancel()
			}
			done := make(chan error, 1)
			go func() { _, e := s.scanBind(ctx, volumetypes.BindKey("/srv/app.conf")); done <- e }()
			<-entered
			if !deadline {
				cancel()
			}
			err = <-done
			if deadline {
				var p *dto.Problem
				require.ErrorAs(t, err, &p)
				require.Equal(t, 504, p.Status)
				require.Equal(t, "volume.bind.size-timeout", p.Code)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			select {
			case <-removed:
			default:
				t.Fatal("scan returned without removing its helper")
			}
		})
	}
}
