// Source: docklet-next/issues/386
// Revision: dda7c807a3cdd6a46d2d3735c43f57447dedaa61
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original archive compatibility proof of the streamed download producer (#386 review R1 B2).

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/stretchr/testify/require"
)

// archiveEntry is everything a restore reads from a tar header plus the content digest.
type archiveEntry struct {
	Name, Link, Digest string
	Type               byte
	Mode, Size         int64
	UID, GID           int
	ModTime            int64
}

func archiveEntries(t *testing.T, gzipped []byte) []archiveEntry {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(gunzip(t, gzipped)))
	entries := []archiveEntry{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries
		}
		require.NoError(t, err)
		hash := sha256.New()
		_, err = io.Copy(hash, reader)
		require.NoError(t, err)
		entries = append(entries, archiveEntry{Name: header.Name, Link: header.Linkname, Digest: hex.EncodeToString(hash.Sum(nil)), Type: header.Typeflag, Mode: header.Mode, Size: header.Size, UID: header.Uid, GID: header.Gid, ModTime: header.ModTime.Unix()})
	}
}

// localTar runs the helper's tar command on this machine, mapping the helper's /scratch tree into root.
func localTar(t *testing.T, root string) func(context.Context, string, []string, io.Writer) error {
	t.Helper()
	binary, err := exec.LookPath("tar")
	require.NoError(t, err, "the archive compatibility proof runs the system tar")
	return func(ctx context.Context, _ string, command []string, stdout io.Writer) error {
		args := make([]string, 0, len(command)-1)
		for _, arg := range command[1:] {
			if strings.HasPrefix(arg, "/scratch") {
				arg = filepath.Join(root, arg)
			}
			args = append(args, arg)
		}
		run := exec.CommandContext(ctx, binary, args...)
		run.Stdout = stdout
		return run.Run()
	}
}

func writeFixture(t *testing.T, data, kind string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(data, 0o755))
	file := func(name, body string, mode os.FileMode) {
		path := filepath.Join(data, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), mode))
		require.NoError(t, os.Chmod(path, mode))
	}
	switch kind {
	case "named-volume":
		file("config/app.conf", "listen 8080\n", 0o640)
		file("data/empty", "", 0o600)
		file("data/blob.bin", strings.Repeat("\x00\x01binary", 4096), 0o644)
		require.NoError(t, os.Symlink("config/app.conf", filepath.Join(data, "current")))
		require.NoError(t, os.Link(filepath.Join(data, "config/app.conf"), filepath.Join(data, "data/app.hard")))
		require.NoError(t, os.Mkdir(filepath.Join(data, "spool"), 0o700))
	case "directory-bind":
		file("site/index.html", "<h1>example</h1>\n", 0o644)
		file("site/assets/run.sh", "#!/bin/sh\necho ok\n", 0o755)
		require.NoError(t, os.Symlink("../outside", filepath.Join(data, "site/external")))
	case "file-bind":
		file("settings.json", "{\"example\":true}\n", 0o600)
	}
}

// The streamed archive is the tar of /scratch/data that the earlier `tar -czf` producer packed, entry for entry,
// and the upload restore accepts it.
func TestStreamedDownloadArchiveMatchesEarlierProducer(t *testing.T) {
	binary, err := exec.LookPath("tar")
	require.NoError(t, err)
	for _, kind := range []string{"empty", "named-volume", "directory-bind", "file-bind"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			data := filepath.Join(root, "scratch", "data")
			writeFixture(t, data, kind)
			r := newStreamRig(t, "h1")
			r.f.service.deps.ExecStream = localTar(t, root)
			response := r.get(t.Context())
			require.Equal(t, http.StatusOK, response.StatusCode)
			streamed, err := readStream(response).finish(t)
			require.NoError(t, err)
			earlier, err := exec.CommandContext(t.Context(), binary, "-czf", "-", "-C", data, ".").Output()
			require.NoError(t, err)
			want := archiveEntries(t, earlier)
			require.Equal(t, want, archiveEntries(t, streamed))
			validation, err := restore.ValidateArchive(bytes.NewReader(streamed), 1<<30)
			require.NoError(t, err, "the upload restore must accept a downloaded archive")
			require.Equal(t, len(want), validation.Entries)
			var normalized bytes.Buffer
			require.NoError(t, restore.NormalizeArchive(bytes.NewReader(streamed), &normalized, 1<<30, validation.SHA256))
			var reencoded bytes.Buffer
			zw := gzip.NewWriter(&reencoded)
			_, err = zw.Write(normalized.Bytes())
			require.NoError(t, err)
			require.NoError(t, zw.Close())
			require.Len(t, archiveEntries(t, reencoded.Bytes()), len(want), fmt.Sprintf("normalized %s archive keeps every entry", kind))
			r.requireReleased()
		})
	}
}
