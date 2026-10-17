//go:build linux

// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOnboardingChecksEffectiveIdentity(t *testing.T) {
	directory := t.TempDir()
	checks := inspectOnboarding(directory, "unix://"+filepath.Join(directory, "absent.sock"))
	require.Equal(t, directory, checks.ProjectDirectory)
	require.NotNil(t, checks.OwnerUID)
	require.NotNil(t, checks.OwnerGID)
	require.NotNil(t, checks.Writable)
	require.True(t, *checks.Writable)
	require.Nil(t, checks.SocketGroup)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries)
	missing := inspectOnboarding(filepath.Join(directory, "missing"), "tcp://agent.example.com:2376")
	require.Nil(t, missing.OwnerUID)
	require.Nil(t, missing.OwnerGID)
	require.Nil(t, missing.Writable)
	require.Nil(t, missing.SocketGroup)
}
