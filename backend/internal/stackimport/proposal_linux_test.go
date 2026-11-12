// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestProposalFileAccessSpy(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{"input.env": "VALUE=private-value\n", "input.labels": "private-label=private-value\n", "include.yaml": "services: {injected: {image: injected:1}}\n", "extends.yaml": "services: {base: {image: injected:1}}\n"}
			if present {
				for name, data := range files {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0600))
				}
			}
			fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
			require.NoError(t, err)
			defer unix.Close(fd)
			_, err = unix.InotifyAddWatch(fd, dir, unix.IN_OPEN|unix.IN_ACCESS)
			require.NoError(t, err)
			data := fmt.Sprintf("include: [%q]\nservices:\n  app:\n    image: alpine:3\n    env_file: [%q]\n    label_file: [%q]\n    extends: {file: %q, service: base}\n    environment: {VALUE: '${REQUIRED:?x}'}\n", filepath.Join(dir, "include.yaml"), filepath.Join(dir, "input.env"), filepath.Join(dir, "input.labels"), filepath.Join(dir, "extends.yaml"))
			result, err := parseProposal(t.Context(), []byte(data))
			require.NoError(t, err)
			require.Equal(t, data, result.ComposeContent)
			require.Equal(t, []string{"REQUIRED"}, result.EnvNames)
			require.Contains(t, result.Missing, "import.missing.label-file")
			require.Contains(t, result.Missing, "import.missing.env-file")
			require.Contains(t, result.Missing, "import.missing.include")
			require.Contains(t, result.Missing, "import.missing.extends")
			n, err := unix.Read(fd, make([]byte, 8192))
			require.ErrorIs(t, err, unix.EAGAIN, "no input file may be opened or read")
			require.Equal(t, -1, n)
		})
	}
}
func TestProposalLabelFileOnly(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "input.labels")
			if present {
				require.NoError(t, os.WriteFile(file, []byte("private-label=secret\n"), 0600))
			}
			fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
			require.NoError(t, err)
			defer unix.Close(fd)
			_, err = unix.InotifyAddWatch(fd, dir, unix.IN_OPEN|unix.IN_ACCESS)
			require.NoError(t, err)
			data := fmt.Sprintf("services:\n  app:\n    image: alpine:3\n    label_file: [%q]\n", file)
			result, err := parseProposal(t.Context(), []byte(data))
			require.NoError(t, err)
			require.Equal(t, data, result.ComposeContent)
			require.Equal(t, []string{"import.missing.label-file"}, result.Missing)
			n, err := unix.Read(fd, make([]byte, 8192))
			require.ErrorIs(t, err, unix.EAGAIN)
			require.Equal(t, -1, n)
		})
	}
}
