// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package workspace

import (
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// The exec fake uses a PATH containing only shipped applets. Shell builtins stay available.
func bindAppletPath(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../../../deploy/tools/applets.txt")
	require.NoError(t, err)
	dir := t.TempDir()
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		binary, err := exec.LookPath(line)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(binary, filepath.Join(dir, line)))
	}
	return dir
}
func TestBindFileWritePreservesInodeAndRecoversPartialWrites(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []byte
		fail    bool
	}{{"nonempty", []byte("replacement"), false}, {"empty", []byte{}, false}, {"binary", []byte{0, 255, 1, 0}, false}, {"partial-write", []byte("replacement"), true}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			staging := filepath.Join(root, "staging")
			require.NoError(t, os.Mkdir(staging, 0700))
			live := filepath.Join(root, "app.conf")
			staged := filepath.Join(staging, "change-0")
			original := []byte("original contents")
			require.NoError(t, os.WriteFile(live, original, 0640))
			require.NoError(t, os.WriteFile(staged, tc.content, 0600))
			before, err := os.Stat(live)
			require.NoError(t, err)
			script := strings.ReplaceAll(bindFileWriteScript, "/tmp/arcane-workspace", staging)
			if tc.fail {
				script = `head() {
if test "$3" = "$4"; then return 1; fi
if test "$3" = "` + staged + `"; then command head -c 2 "$3"; return 1; fi
command head "$@"
}
` + script
			}
			command := exec.CommandContext(t.Context(), "/bin/sh", "-c", script, "sh", live, staged, strconv.Itoa(len(tc.content)))
			command.Env = []string{"PATH=" + bindAppletPath(t)}
			output, err := command.CombinedOutput()
			if tc.fail {
				require.Error(t, err, string(output))
			} else {
				require.NoError(t, err, string(output))
			}
			actual, err := os.ReadFile(live)
			require.NoError(t, err)
			if tc.fail {
				require.Equal(t, original, actual)
			} else {
				require.Equal(t, tc.content, actual)
			}
			after, err := os.Stat(live)
			require.NoError(t, err)
			a, b := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
			require.Equal(t, a.Ino, b.Ino)
			require.Equal(t, a.Uid, b.Uid)
			require.Equal(t, a.Gid, b.Gid)
			require.Equal(t, before.Mode(), after.Mode())
		})
	}
}
func TestBindFileManifestRefusesBeforeStaging(t *testing.T) {
	service := &Service{}
	key := volume.BindKey("/srv/app.conf")
	for _, change := range []volume.WorkspaceFileChange{{Operation: volume.FileOpDelete, RelativePath: "app.conf"}, {Operation: volume.FileOpCreateFile, RelativePath: "app.conf"}, {Operation: volume.FileOpUpdateFile, RelativePath: "other"}, {Operation: volume.FileOpRename, RelativePath: "app.conf", NewName: "other"}} {
		_, err := service.updateBindFile(t.Context(), "helper", key, volume.WorkspaceUpdateManifest{FileChanges: []volume.WorkspaceFileChange{change}}, nil)
		require.EqualError(t, err, "volume.bind.file-operation-unsupported")
	}
}
