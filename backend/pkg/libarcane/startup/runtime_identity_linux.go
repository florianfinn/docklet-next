// Source: getarcaneapp/arcane/backend/pkg/libarcane/startup/runtime_identity_linux.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Preserve start groups and forward shutdown during identity re-exec.

//go:build linux

package startup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/samber/mo"
)

func reexecWithRuntimeIdentityInternal(ctx context.Context, req runtimeIdentityRequest) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	startGroups, err := os.Getgroups()
	if err != nil {
		return fmt.Errorf("read runtime start groups: %w", err)
	}
	initial := make([]uint32, 0, len(startGroups))
	for _, gid := range startGroups {
		initial = append(initial, uint32(gid))
	}
	groups := runtimeIdentitySupplementaryGroupsInternal(req.DockerHost, resolveSocketGroupInternal, initial...)

	cmd := exec.CommandContext(ctx, executable, os.Args[1:]...) //nolint:gosec // re-executing our own binary with the same args under a different UID/GID
	// Forward shutdown instead of the default SIGKILL so the child can unregister its actor host.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    req.CredentialUID,
			Gid:    req.CredentialGID,
			Groups: groups,
		},
	}

	if startErr := cmd.Start(); startErr != nil {
		return fmt.Errorf("start runtime identity child: %w", startErr)
	}

	waitErr := cmd.Wait()
	// After a forwarded shutdown Wait reports ctx.Err() even when the child exited cleanly.
	if cmd.ProcessState != nil && cmd.ProcessState.Success() {
		os.Exit(0)
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		if status, hasStatus := exitErr.Sys().(syscall.WaitStatus); hasStatus {
			if status.Signaled() {
				os.Exit(128 + int(status.Signal()))
			}
			os.Exit(status.ExitStatus())
		}
		os.Exit(exitErr.ExitCode())
	}

	return fmt.Errorf("wait for runtime identity child: %w", waitErr)
}

func resolveSocketGroupInternal(socketPath string) mo.Option[uint32] {
	// os.* rather than acfs: the docker socket is a host-configured system path
	// probed during identity bootstrap, not under any acfs root.
	info, err := os.Stat(socketPath)
	if err != nil {
		return mo.None[uint32]()
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return mo.None[uint32]()
	}

	return mo.Some(stat.Gid)
}
