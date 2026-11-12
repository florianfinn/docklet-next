// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	engine "go.getarcane.app/updater"
)

type protectedPull struct{ client *client.Client }

func (p protectedPull) PullImage(ctx context.Context, ref string, _ io.Writer) error {
	stream, err := p.client.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(io.Discard, stream)
	return err
}
func TestSelfTargetDeclinedBeforeEngineMutations(t *testing.T) {
	for _, role := range []string{"own-id", "hub", "agent"} {
		t.Run(role, func(t *testing.T) {
			var mutations atomic.Int32
			labels := map[string]string{}
			selfID := "current"
			if role != "own-id" {
				labels["io.docklet.system"] = role
				selfID = "another-container"
			}
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Api-Version", "1.56")
				path := r.URL.Path
				if strings.HasPrefix(path, "/v1.") {
					_, path, _ = strings.Cut(path[1:], "/")
					path = "/" + path
				}
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					mutations.Add(1)
				}
				switch {
				case path == "/_ping":
					_, _ = w.Write([]byte("OK"))
				case path == "/containers/json":
					_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "current", "Names": []string{"/docklet"}, "Image": "example.com/docklet:1.0.0", "Labels": labels}})
				case path == "/containers/current/json":
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": "current", "Image": "sha256:old", "Config": map[string]any{"Image": "example.com/docklet:1.0.0", "Labels": labels}, "HostConfig": map[string]any{}, "State": map[string]any{"Status": "running", "Running": true}})
				case path == "/images/create":
					_, _ = w.Write([]byte("{}\n"))
				case strings.HasPrefix(path, "/images/"):
					_, _ = w.Write([]byte(`{"Id":"sha256:new","RepoTags":["example.com/docklet:1.0.0"]}`))
				default:
					http.Error(w, "unexpected Engine operation", 500)
				}
			}))
			defer receiver.Close()
			cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(receiver.URL, "http://")), client.WithAPIVersion("1.56"))
			require.NoError(t, err)
			defer cli.Close()
			service, err := engine.New(engine.Config{DockerClientProvider: fakeDockerClientProviderInternal{client: cli}, ImagePuller: protectedPull{client: cli}, SelfContainerID: selfID, LabelPolicy: engine.DefaultLabelPolicy()})
			require.NoError(t, err)
			defer service.Close()
			result, err := service.UpdateContainer(t.Context(), "current", engine.Options{Force: true})
			require.NoError(t, err)
			require.Equal(t, 1, result.Failed)
			require.Len(t, result.Items, 1)
			require.Contains(t, result.Items[0].Error, "image.system-protected")
			require.Zero(t, mutations.Load(), "Docklet must be declined before pull, create, stop or remove")
		})
	}
}
