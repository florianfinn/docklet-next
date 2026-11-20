// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package hostio

import (
	"context"
	"time"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
)

// The fallback hashes the same first 64 KiB as the helper, while consuming and
// discarding excess bytes until wget exits or its request budget expires.
type probeBodyBuffer struct{ data []byte }

func (b *probeBodyBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := MaxProbeBody + 1 - len(b.data); remaining > 0 {
		b.data = append(b.data, p[:min(remaining, n)]...)
	}
	return n, nil
}
func WgetProbe(ctx context.Context, cli *client.Client, id string, kind dto.Kind, host, rawURL string) (Response, error) {
	args, _, err := Args(kind, "probe", "", host, rawURL)
	if err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stdout := &probeBodyBuffer{}
	stderr := &boundedBuffer{limit: MaxProbeStderr}
	exit, err := docker.ExecInContainer(ctx, cli, id, client.ExecCreateOptions{AttachStdout: true, AttachStderr: true, Cmd: args}, stdout, stderr)
	if ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	return ParseProbe(ExecResult{Stdout: stdout.data, Stderr: stderr.Bytes(), ExitCode: exit}, err)
}
