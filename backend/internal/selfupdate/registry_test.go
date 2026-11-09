// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"context"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func registryImage(t *testing.T, arch, version string) v1.Image {
	t.Helper()
	image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: arch, Config: v1.Config{Labels: map[string]string{"version": version}}})
	require.NoError(t, err)
	return image
}
func TestExactTagManifestDigestBindsEveryConsumer(t *testing.T) {
	receiver := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer receiver.Close()
	repo := strings.TrimPrefix(receiver.URL, "http://") + "/docklet-hub"
	oldTag, err := name.NewTag(repo + ":1.0.0")
	require.NoError(t, err)
	targetTag, err := name.NewTag(repo + ":1.0.1")
	require.NoError(t, err)
	oldImage := registryImage(t, "amd64", "old")
	target := registryImage(t, "amd64", "target")
	require.NoError(t, remote.Write(oldTag, oldImage))
	require.NoError(t, remote.Write(targetTag, target))
	hash, err := target.Digest()
	require.NoError(t, err)
	pin, err := ResolveTarget(t.Context(), repo, "1.0.1", receiver.Client().Transport)
	require.NoError(t, err)
	require.Equal(t, repo+":1.0.1@"+hash.String(), pin)
	data, request, _ := requestFixture(t)
	request.TargetReference = pin
	require.NoError(t, WriteJSON(RequestPath(data, request.OpID), request))
	create := HelperCreate(request, nil)
	require.Equal(t, pin, create.Config.Image)
	helper := Helper{Up: func(_ context.Context, p *composetypes.Project, svc string) error {
		require.Equal(t, pin, p.Services[svc].Image)
		env, err := os.ReadFile(request.Replay.EnvFile)
		require.NoError(t, err)
		require.Contains(t, string(env), "HUB_IMAGE="+pin)
		return nil
	}, Healthy: func(context.Context, Request) error { return nil }}
	require.NoError(t, helper.Run(t.Context(), data, request.OpID))
	moved := registryImage(t, "amd64", "moved-tag")
	require.NoError(t, remote.Write(targetTag, moved))
	movedHash, err := moved.Digest()
	require.NoError(t, err)
	after, err := ResolveTarget(t.Context(), repo, "1.0.1", receiver.Client().Transport)
	require.NoError(t, err)
	require.Equal(t, repo+":1.0.1@"+movedHash.String(), after)
	require.NotEqual(t, pin, after)
	missing := registryImage(t, "arm64", "unsupported")
	require.NoError(t, remote.Write(targetTag, missing))
	_, err = ResolveTarget(t.Context(), repo, "1.0.1", receiver.Client().Transport)
	require.Error(t, err)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: missing, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}})
	require.NoError(t, remote.WriteIndex(targetTag, index))
	_, err = ResolveTarget(t.Context(), repo, "1.0.1", receiver.Client().Transport)
	require.Error(t, err)
	index = mutate.AppendManifests(index, mutate.IndexAddendum{Add: target, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}})
	require.NoError(t, remote.WriteIndex(targetTag, index))
	digest, err := index.Digest()
	require.NoError(t, err)
	after, err = ResolveTarget(t.Context(), repo, "1.0.1", receiver.Client().Transport)
	require.NoError(t, err)
	require.Equal(t, repo+":1.0.1@"+digest.String(), after)
}
