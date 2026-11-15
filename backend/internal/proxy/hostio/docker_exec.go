// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original bounded Docker commands, logs and restart adapters.

package hostio

import (
	"context"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
)

// DockerExecutor bounds writes while the process runs, rather than after an
// unbounded buffer has already consumed memory. Exit status remains separate.
func DockerExecutor(cli *client.Client, id string) Executor {
	return func(ctx context.Context, args []string, limit int) (ExecResult, error) {
		out := &boundedBuffer{limit: limit}
		stderr := &boundedBuffer{limit: limit}
		exit, err := docker.ExecInContainer(ctx, cli, id, client.ExecCreateOptions{AttachStdout: true, AttachStderr: true, Cmd: args}, out, stderr)
		return ExecResult{Stdout: out.Bytes(), Stderr: stderr.Bytes(), ExitCode: exit}, err
	}
}
