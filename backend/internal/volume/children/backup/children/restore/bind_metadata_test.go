// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package restore

import (
	"archive/tar"
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadRestorePreservesNestedNumericMetadata(t *testing.T) {
	scratch, root := t.TempDir(), t.TempDir()
	file, err := os.Create(filepath.Join(scratch, ".docklet-copy.tar"))
	require.NoError(t, err)
	archive := tar.NewWriter(file)
	for _, header := range []*tar.Header{{Name: "nested/", Typeflag: tar.TypeDir, Mode: 0755, Uid: os.Geteuid(), Gid: os.Getegid()}, {Name: "nested/data", Typeflag: tar.TypeReg, Size: 4, Mode: 0644, Uid: os.Geteuid(), Gid: os.Getegid()}} {
		require.NoError(t, archive.WriteHeader(header))
		if header.Size > 0 {
			_, err = archive.Write([]byte("data"))
			require.NoError(t, err)
		}
	}
	require.NoError(t, archive.Close())
	require.NoError(t, file.Close())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/containers/create") {
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "writer"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/start") {
			w.WriteHeader(204)
			return
		}
		t.Errorf("unexpected %s", r.URL.Path)
		w.WriteHeader(500)
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer cli.Close()
	s := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli), HelperImage: func(context.Context, *client.Client) (string, error) { return "tools:1.0.0", nil }, Exec: func(ctx context.Context, _, _ string, args []string) (string, string, error) {
		require.Equal(t, "sh", args[0])
		script := strings.ReplaceAll(strings.ReplaceAll(args[2], "/volume", root), "/scratch", scratch)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "umask 077; "+script)
		out, err := cmd.CombinedOutput()
		return string(out), "", err
	}})
	run := &Run{VolumeName: "app", Prepared: &preparedRestore{scratch: "scratch"}}
	require.NoError(t, s.WriteRestore(t.Context(), run, StartRequest{VolumeName: "app", Kind: "upload"}))
	for name, mode := range map[string]os.FileMode{"nested": 0755, "nested/data": 0644} {
		stat, err := os.Stat(filepath.Join(root, name))
		require.NoError(t, err)
		require.Equal(t, mode, stat.Mode().Perm())
	}
	data, err := os.ReadFile(filepath.Join(root, "nested/data"))
	require.NoError(t, err)
	require.Equal(t, "data", string(data))
}
