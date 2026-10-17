//go:build linux

// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package version

import (
	"os"
	"strings"
	"syscall"

	"github.com/florianfinn/docklet-next/types/version"
)

func inspectOnboarding(projectDirectory, dockerHost string) *version.OnboardingChecks {
	checks := &version.OnboardingChecks{ProjectDirectory: projectDirectory}
	if info, err := os.Stat(projectDirectory); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			checks.OwnerUID = new(stat.Uid)
			checks.OwnerGID = new(stat.Gid)
		}
		writable := false
		if info.IsDir() {
			file, err := os.CreateTemp(projectDirectory, ".docklet-write-check-*")
			if err == nil {
				name := file.Name()
				closeErr := file.Close()
				removeErr := os.Remove(name)
				writable = closeErr == nil && removeErr == nil
			}
		}
		checks.Writable = &writable
	}
	if socketPath, ok := strings.CutPrefix(dockerHost, "unix://"); ok {
		if info, err := os.Stat(socketPath); err == nil && info.Mode()&os.ModeSocket != 0 {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				checks.SocketGroup = new(stat.Gid)
			}
		}
	}
	return checks
}
