// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestR14WriterCASAndMetadata(t *testing.T) {
	for _, branch := range []string{"third", "symlink", "hardlink", "xattr", "world-writable"} {
		t.Run(branch, func(t *testing.T) {
			dir := t.TempDir()
			p := ".env"
			require.NoError(t, os.WriteFile(filepath.Join(dir, p), []byte("OLD=1\n"), 0600))
			old, err := resolver.Read(dir, p)
			require.NoError(t, err)
			switch branch {
			case "third":
				require.NoError(t, os.WriteFile(filepath.Join(dir, p), []byte("THIRD=1\n"), 0600))
			case "symlink":
				require.NoError(t, os.Rename(filepath.Join(dir, p), filepath.Join(dir, "actual")))
				require.NoError(t, os.Symlink("actual", filepath.Join(dir, p)))
			case "hardlink":
				require.NoError(t, os.Link(filepath.Join(dir, p), filepath.Join(dir, "other")))
			case "xattr":
				require.NoError(t, unix.Setxattr(filepath.Join(dir, p), "user.example", []byte("present"), 0))
			case "world-writable":
				require.NoError(t, os.Chmod(filepath.Join(dir, p), 0666))
			}
			err = resolver.Replace(dir, p, old.Old, []byte("NEW=1\n"), true, 0600)
			require.Error(t, err)
			data, err := os.ReadFile(filepath.Join(dir, p))
			require.NoError(t, err)
			require.NotEqual(t, "NEW=1\n", string(data))
		})
	}
}
func TestSecretWriterPreservesOwnerModeAndCreates0600(t *testing.T) {
	dir := t.TempDir()
	err := resolver.Replace(dir, ".env", dto.FileState{}, []byte("VALUE='literal'\n"), true, 0600)
	require.NoError(t, err)
	st, err := os.Stat(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), st.Mode().Perm())
	require.NoError(t, os.Chmod(filepath.Join(dir, ".env"), 0400))
	before, err := resolver.Read(dir, ".env")
	require.NoError(t, err)
	require.NoError(t, resolver.Replace(dir, ".env", before.Old, []byte("VALUE='changed'\n"), true, before.Mode))
	st, err = os.Stat(filepath.Join(dir, ".env"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), st.Mode().Perm())
}
