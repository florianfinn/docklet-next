// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original bounded Docker commands, logs and restart adapters.

package proxy

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"io"
	"time"
)

// InspectContainer is internal engine input; engines project only allowlisted
// leaves into DTOs. Raw inspect/environment/commands never leave the host API.
func (d *DockerRuntime) InspectContainer(ctx context.Context, i dto.Instance) (container.InspectResponse, error) {
	if err := d.Check(ctx, i); err != nil {
		return container.InspectResponse{}, err
	}
	return d.inspect(ctx, i.Identity.ContainerName)
}
func (d *DockerRuntime) Exec(ctx context.Context, i dto.Instance, action, file, host, rawURL string) (hostio.ExecResult, error) {
	if err := d.Check(ctx, i); err != nil {
		return hostio.ExecResult{}, err
	}
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return hostio.ExecResult{}, err
	}
	return hostio.Exec(ctx, hostio.DockerExecutor(cli, i.Identity.ContainerID), i.Kind, action, file, host, rawURL)
}
func (d *DockerRuntime) RestartContainer(ctx context.Context, i Intent) (string, error) {
	if err := d.Check(ctx, i.Request.Instance); err != nil {
		return "", err
	}
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return "", err
	}
	return hostio.Restart(ctx, i.Restart && i.Request.Instance.Kind == dto.Caddy && i.Request.Acknowledgements.Restart, d.epochInspector(i.Request.Instance), func(ctx context.Context) error {
		_, err := cli.ContainerRestart(ctx, i.Request.Instance.Identity.ContainerID, client.ContainerRestartOptions{})
		return err
	})
}
func (d *DockerRuntime) epochInspector(i dto.Instance) hostio.Inspect {
	return func(ctx context.Context) (hostio.Epoch, error) {
		c, err := d.inspect(ctx, i.Identity.ContainerName)
		if err != nil {
			return hostio.Epoch{}, err
		}
		if c.State == nil {
			return hostio.Epoch{}, dto.Fail(dto.ContainerChanged, 409)
		}
		return hostio.Epoch{ContainerID: c.ID, StartedAt: c.State.StartedAt, Running: c.State.Running}, nil
	}
}

// Logs uses the existing Docker demultiplexer. Each consumed batch and the final
// verdict have an epoch check; missing/rotated/file-driver logs are unavailable.
func (d *DockerRuntime) Logs(ctx context.Context, i dto.Instance, since time.Time, consume func(string) error) error {
	c, err := d.InspectContainer(ctx, i)
	if err != nil {
		return err
	}
	if c.State == nil || c.HostConfig == nil || c.HostConfig.LogConfig.Type == "none" {
		return dto.Fail(dto.LogsUnavailable, 409)
	}
	fence := hostio.LogFence{Expected: hostio.Epoch{ContainerID: c.ID, StartedAt: c.State.StartedAt}, Inspect: d.epochInspector(i)}
	cli, err := d.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	logs, err := cli.ContainerLogs(ctx, c.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Since: since.Format(time.RFC3339Nano), Timestamps: true})
	if err != nil {
		return dto.Fail(dto.LogsUnavailable, 409)
	}
	defer logs.Close()
	life, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(life, func() { _ = logs.Close() })
	defer stop()
	batches := make(chan string, 1)
	done := make(chan error, 1)
	tty := c.Config != nil && c.Config.Tty
	go func() { defer close(batches); done <- docker.StreamContainerLogs(life, logs, batches, false, tty) }()
	var consumeErr error
	for batch := range batches {
		if consumeErr != nil {
			continue
		}
		if err = fence.Check(ctx); err != nil {
			consumeErr = err
			cancel()
			continue
		}
		if len(batch) > hostio.MaxExecBytes {
			consumeErr = dto.Fail(dto.InputLimit, 422)
			cancel()
			continue
		}
		if err = consume(batch); err != nil {
			consumeErr = err
			cancel()
		}
	}
	readErr := <-done
	if consumeErr != nil {
		return consumeErr
	}
	if readErr != nil && readErr != io.EOF {
		return dto.Fail(dto.LogsUnavailable, 409)
	}
	return fence.Settle(ctx)
}
