// Source: docklet-next/issues/676
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduler regression coverage.

package projects

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreContinuesIndependentDirectoriesFilesAndTopLevel(t *testing.T) {
	project := t.TempDir()
	saved := t.TempDir()
	write := func(root, name, value string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(value), 0600))
	}
	for _, name := range []string{"early/data", "later/data", "individual", "compose.yaml"} {
		write(saved, name, "original")
	}
	for _, name := range []string{"later/data", "compose.yaml", "created"} {
		write(project, name, "changed")
	}
	require.NoError(t, os.RemoveAll(filepath.Join(saved, "early")))
	require.NoError(t, os.Remove(filepath.Join(saved, "individual")))
	b := &ProjectUpdateBackup{BackupDir: saved, DirEntries: []string{"early", "later"}, FileEntries: []string{"individual"}, TopLevelFiles: true}
	err := RestoreProjectUpdateBackup(t.Context(), project, b)
	require.Error(t, err)
	require.Contains(t, err.Error(), "early")
	require.Contains(t, err.Error(), "individual")
	for _, name := range []string{"later/data", "compose.yaml"} {
		data, e := os.ReadFile(filepath.Join(project, name))
		require.NoError(t, e)
		require.Equal(t, "original", string(data))
	}
	_, err = os.Stat(filepath.Join(project, "created"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
func TestTopLevelRestoreContinuesAfterFileFailure(t *testing.T) {
	project := t.TempDir()
	saved := t.TempDir()
	for _, name := range []string{".env", "z.yaml"} {
		require.NoError(t, os.WriteFile(filepath.Join(saved, name), []byte("original"), 0600))
	}
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "absent"), filepath.Join(project, ".env")))
	b := &ProjectUpdateBackup{BackupDir: saved, TopLevelFiles: true, envSymlink: &projectUpdateEnvSymlinkBackup{relativePath: ".env", linkTarget: "previous"}}
	err := RestoreProjectUpdateBackup(t.Context(), project, b)
	require.Error(t, err)
	data, e := os.ReadFile(filepath.Join(project, "z.yaml"))
	require.NoError(t, e)
	require.Equal(t, "original", string(data))
}

func TestBackupPermissionSitesLogContext(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission refusal requires an unprivileged process")
	}
	for _, site := range []string{"temp", "restore-root", "directory", "parent"} {
		t.Run(site, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			root := t.TempDir()
			project := filepath.Join(root, "project")
			saved := t.TempDir()
			require.NoError(t, os.Mkdir(project, 0755))
			backup := &ProjectUpdateBackup{BackupDir: saved}
			var err error
			switch site {
			case "temp":
				require.NoError(t, os.Chmod(root, 0555))
				defer os.Chmod(root, 0755)
				_, _, err = BackupProjectDirectory(t.Context(), root, project, "backup-*", ProjectUpdateBackupScope{})
			case "restore-root":
				require.NoError(t, os.Remove(project))
				require.NoError(t, os.Chmod(root, 0555))
				defer os.Chmod(root, 0755)
				err = RestoreProjectDirectoryBackup(t.Context(), root, project, backup)
			case "directory":
				require.NoError(t, os.Chmod(project, 0555))
				defer os.Chmod(project, 0755)
				backup.DirEntries = []string{"data"}
				err = RestoreProjectUpdateBackup(t.Context(), project, backup)
			case "parent":
				require.NoError(t, os.Mkdir(filepath.Join(saved, "data"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(saved, "data", "config"), []byte("saved"), 0600))
				require.NoError(t, os.Chmod(project, 0555))
				defer os.Chmod(project, 0755)
				backup.FileEntries = []string{"data/config"}
				err = RestoreProjectUpdateBackup(t.Context(), project, backup)
			}
			require.ErrorIs(t, err, os.ErrPermission)
			require.NotContains(t, err.Error(), root)
			require.Contains(t, logs.String(), `"path":`)
			require.Contains(t, logs.String(), fmt.Sprintf(`"uid":%d`, os.Getuid()))
			require.Contains(t, logs.String(), fmt.Sprintf(`"gid":%d`, os.Getgid()))
		})
	}
}

func TestTopLevelPruneFailureDoesNotPreventRestore(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission refusal requires an unprivileged process")
	}
	project := t.TempDir()
	saved := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(saved, "compose.yaml"), []byte("original"), 0600))
	for _, name := range []string{"created", "compose.yaml"} {
		require.NoError(t, os.WriteFile(filepath.Join(project, name), []byte("changed"), 0600))
	}
	require.NoError(t, os.Chmod(project, 0555))
	defer os.Chmod(project, 0755)
	err := RestoreProjectUpdateBackup(t.Context(), project, &ProjectUpdateBackup{BackupDir: saved, TopLevelFiles: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "prune created file created")
	data, e := os.ReadFile(filepath.Join(project, "compose.yaml"))
	require.NoError(t, e)
	require.Equal(t, "original", string(data))
}
