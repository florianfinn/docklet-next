// Source: getarcaneapp/arcane/backend/internal/container/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split lifecycle, editing, inventory and stream concerns; exclude Docklet agent inventory.

package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
}

func (e *ExecSession) Stdin() io.WriteCloser { return e.hijackedResp.Conn }

func (e *ExecSession) Stdout() io.Reader { return e.hijackedResp.Reader }

// Close terminates the exec session and kills the process if still running.
func (e *ExecSession) Close(ctx context.Context) error {
	var closeErr error
	e.closeOnce.Do(func() {
		slog.DebugContext(ctx, "Closing exec session", "execId", e.execID, "containerId", e.containerID)

		// Send EOF (Ctrl-D) then exit to terminate the shell gracefully.
		_, _ = e.hijackedResp.Conn.Write([]byte{0x04})
		time.Sleep(50 * time.Millisecond)
		_, _ = e.hijackedResp.Conn.Write([]byte("exit\n"))
		time.Sleep(100 * time.Millisecond)

		e.hijackedResp.Close()
	})

	return closeErr
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
