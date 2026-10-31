// Source: getarcaneapp/arcane/backend/internal/container/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split lifecycle, editing, inventory and stream concerns; exclude Docklet agent inventory; M14 exec resize, inspect and orphan cleanup.

package container

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"io"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
)

// CreateExec creates an exec instance in the container
func (s *ContainerService) CreateExec(ctx context.Context, containerID string, cmd []string) (string, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Docker: %w", err)
	}

	execConfig := client.ExecCreateOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		Cmd:          cmd,
	}

	execResp, err := dockerClient.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	return execResp.ID, nil
}

// ExecSession manages the lifecycle of a Docker exec session.
type ExecSession struct {
	execID       string
	containerID  string
	hijackedResp client.HijackedResponse
	dockerClient *client.Client
	closeOnce    sync.Once
	closeErr     error
}

func (e *ExecSession) Stdin() io.WriteCloser { return e.hijackedResp.Conn }

func (e *ExecSession) Stdout() io.Reader { return e.hijackedResp.Reader }

// Close requests graceful shell exit, inspects after two seconds and closes
// the hijack even if the process ignored exit. Docker cannot kill an exec alone.
func (e *ExecSession) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		_ = e.hijackedResp.Conn.SetWriteDeadline(time.Now().Add(time.Second))
		_, _ = e.hijackedResp.Conn.Write([]byte{0x04})
		_, _ = e.hijackedResp.Conn.Write([]byte("exit\n"))
		timer := sessionlife.NewTimer(ctx, 2*time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
		state, err := e.dockerClient.ExecInspect(ctx, e.execID, client.ExecInspectOptions{})
		if err != nil {
			e.closeErr = err
		} else if state.Running {
			e.closeErr = errShellOrphaned
		}
		e.hijackedResp.Close()
	})
	return e.closeErr
}
func (e *ExecSession) Resize(ctx context.Context, cols, rows int) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := e.dockerClient.ExecResize(ctx, e.execID, client.ExecResizeOptions{Width: uint(cols), Height: uint(rows)})
	return err
}
func (e *ExecSession) Inspect(ctx context.Context) (client.ExecInspectResult, error) {
	return e.dockerClient.ExecInspect(ctx, e.execID, client.ExecInspectOptions{})
}

// AttachExec attaches to an exec instance and returns an ExecSession for lifecycle management.
func (s *ContainerService) AttachExec(ctx context.Context, containerID, execID string) (*ExecSession, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	execAttach, err := dockerClient.ExecAttach(ctx, execID, client.ExecAttachOptions{
		TTY: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to exec: %w", err)
	}

	return &ExecSession{
		execID:       execID,
		containerID:  containerID,
		hijackedResp: execAttach.HijackedResponse,
		dockerClient: dockerClient,
	}, nil
}

type demuxedLogsInternal struct {
	io.Reader

	pipe *io.PipeReader
	logs io.ReadCloser
}

func (d *demuxedLogsInternal) Close() error {
	return errors.Join(d.logs.Close(), d.pipe.Close())
}

func (s *ContainerService) openLogsInternal(ctx context.Context, containerID string, options client.ContainerLogsOptions) (io.ReadCloser, client.ContainerInspectResult, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, client.ContainerInspectResult{}, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containerInspect, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, client.ContainerInspectResult{}, fmt.Errorf("failed to inspect container for logs: %w", err)
	}

	logs, err := dockerClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return nil, client.ContainerInspectResult{}, fmt.Errorf("failed to get container logs: %w", err)
	}
	return logs, containerInspect, nil
}

func (s *ContainerService) StreamLogs(ctx context.Context, containerID string, logsChan chan<- string, follow bool, tail, since string, timestamps bool) error {
	logs, containerInspect, err := s.openLogsInternal(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Since:      since,
		Timestamps: timestamps,
	})
	if err != nil {
		return err
	}
	defer func() { _ = logs.Close() }()

	isTTY := containerInspect.Container.Config != nil && containerInspect.Container.Config.Tty
	return docker.StreamContainerLogs(ctx, logs, logsChan, follow, isTTY)
}

// DownloadLogs returns every log line Docker retains for the container as a
// single plain-text stream plus the attachment filename. Callers must Close it.
func (s *ContainerService) DownloadLogs(ctx context.Context, containerID string) (io.ReadCloser, string, error) {
	logs, containerInspect, err := s.openLogsInternal(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "all",
		Timestamps: true,
	})
	if err != nil {
		return nil, "", err
	}

	filename := "container-" + containerInspect.Container.ID[:min(12, len(containerInspect.Container.ID))] + "-logs.log"
	if containerInspect.Container.Config != nil && containerInspect.Container.Config.Tty {
		return logs, filename, nil
	}

	pr, pw := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(pw, pw, logs)
		if docker.IsExpectedStreamEndError(copyErr) {
			copyErr = nil
		}
		_ = pw.CloseWithError(copyErr)
	}()
	return &demuxedLogsInternal{Reader: pr, pipe: pr, logs: logs}, filename, nil
}
