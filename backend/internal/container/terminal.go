// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell frame protocol, Docker resize and joined process cleanup;
// a missing /bin/sh is named on every resize and stream end path.

package container

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/containerd/errdefs"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/types/container"
	"github.com/moby/moby/client"
)

var errShellProtocol = errors.New("container.shell-protocol")
var errShellOrphaned = errors.New("container.shell-orphaned")

func shellCommand(shell string) []string {
	command := "exec " + shell
	if shell == "auto" {
		command = "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"
	}
	return []string{"/bin/sh", "-c", command}
}

// ServeTerminal runs the agent/local endpoint. Only the hub owns audit identity.
// Reader goroutines remain alive until the final exit has been written: canceling
// coder/websocket.Read before that write would close the socket prematurely.
func (s *ContainerService) ServeTerminal(ctx context.Context, conn *websocket.Conn, containerID string, query container.ShellQuery) {
	reason := ""
	var code *int
	finished := false
	defer func() {
		if finished {
			return
		}
		wshub.WriteTerminalExit(ctx, conn, code, reason)
		status := websocket.StatusNormalClosure
		if reason != "" {
			status = websocket.StatusPolicyViolation
		}
		_ = conn.Close(status, reason)
	}()
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return
	}
	state, err := dockerClient.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return
	}
	if state.Container.State == nil || !state.Container.State.Running {
		reason = "container.shell-not-running"
		return
	}
	execID, err := s.CreateExec(ctx, containerID, shellCommand(query.Shell))
	if err != nil {
		reason = s.shellFailureReason(ctx, containerID, err)
		return
	}
	session, err := s.AttachExec(ctx, containerID, execID)
	if err != nil {
		reason = s.shellFailureReason(ctx, containerID, err)
		return
	}

	if query.Cols != nil || query.Rows != nil {
		cols, rows := 80, 24
		if query.Cols != nil {
			cols = *query.Cols
		}
		if query.Rows != nil {
			rows = *query.Rows
		}
		if err := session.Resize(ctx, cols, rows); err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			// A failed start can end the exec before its first resize; only a live exec needs Close.
			if inspected, err := session.Inspect(cleanup); err == nil && !inspected.Running {
				code = &inspected.ExitCode
				reason = s.endedShellReason(cleanup, containerID, inspected.ExitCode)
				session.hijackedResp.Close()
			} else if errors.Is(session.Close(cleanup), errShellOrphaned) {
				reason = "container.shell-orphaned"
			}
			return
		}
	}
	ioCtx := context.WithoutCancel(ctx)
	var closing atomic.Bool
	inputDone, outputDone := make(chan error, 1), make(chan error, 1)
	go func() { inputDone <- readTerminalInput(ioCtx, conn, session, &closing) }()
	go func() { outputDone <- writeTerminalOutput(ioCtx, conn, session.Stdout()) }()
	inputEnded, outputEnded := false, false
	reason = ""
	select {
	case <-ctx.Done():
		reason = ""
	case err := <-inputDone:
		inputEnded = true
		if err != nil && !wshub.IsExpectedClose(err) {
			reason = ""
			if errors.Is(err, errShellProtocol) {
				reason = "container.shell-protocol"
			}
		}
	case err := <-outputDone:
		outputEnded = true
		if err != nil {
			reason = ""
		}
	}
	closing.Store(true)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	inspected, inspectErr := session.Inspect(cleanupCtx)
	if outputEnded && inspectErr == nil && !inspected.Running {
		code = &inspected.ExitCode
		session.hijackedResp.Close()
		reason = s.endedShellReason(cleanupCtx, containerID, inspected.ExitCode)
	} else {
		if err := session.Close(cleanupCtx); errors.Is(err, errShellOrphaned) {
			reason = "container.shell-orphaned"
		}
		if inspected, err := session.Inspect(cleanupCtx); err == nil && !inspected.Running {
			code = &inspected.ExitCode
			// A resize can fail on an exec whose start already failed, before its stream ends.
			if reason == "" {
				reason = s.endedShellReason(cleanupCtx, containerID, inspected.ExitCode)
			}
		}
	}
	if !outputEnded {
		<-outputDone
	}
	// Finish the browser reader only after sending its authoritative exit frame.
	wshub.WriteTerminalExit(ctx, conn, code, reason)
	status := websocket.StatusNormalClosure
	if reason != "" {
		status = websocket.StatusPolicyViolation
	}
	_ = conn.Close(status, reason)
	if !inputEnded {
		<-inputDone
	}
	// The defer covers early failures; this completed path has already sent exit.
	finished = true
}

func readTerminalInput(ctx context.Context, conn *websocket.Conn, session *ExecSession, closing *atomic.Bool) error {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if closing.Load() {
			continue
		}
		if kind == websocket.MessageBinary {
			_ = session.hijackedResp.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := session.Stdin().Write(data); err != nil {
				return err
			}
			continue
		}
		if wshub.IsTerminalClose(kind, data) {
			return nil
		}
		var resize container.ShellResize
		if kind != websocket.MessageText || json.Unmarshal(data, &resize, json.RejectUnknownMembers(true)) != nil || resize.Type != "resize" || resize.Cols < 1 || resize.Cols > 1000 || resize.Rows < 1 || resize.Rows > 1000 {
			return errShellProtocol
		}
		if err := session.Resize(ctx, resize.Cols, resize.Rows); err != nil {
			return err
		}
	}
}
func writeTerminalOutput(ctx context.Context, conn *websocket.Conn, stdout io.Reader) error {
	buffer := make([]byte, 4096)
	for {
		n, err := stdout.Read(buffer)
		if n > 0 {
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			writeErr := conn.Write(writeCtx, websocket.MessageBinary, buffer[:n])
			cancel()
			if writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (s *ContainerService) shellFailureReason(ctx context.Context, containerID string, failure error) string {
	cli, err := s.dockerService.GetClient(ctx)
	if err == nil {
		state, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
		if err == nil && state.Container.State != nil && !state.Container.State.Running {
			return "container.shell-not-running"
		}
		// ExecAttach discards a non-101 response body in the Docker SDK. Check
		// the executable path rather than inventing a reason from its HTTP code.
		if s.shellMissing(ctx, containerID) {
			return "container.shell-unavailable"
		}
	}
	// Only an explicit missing shell executable proves shell-unavailable.
	message := strings.ToLower(failure.Error())
	if strings.Contains(message, "/bin/sh") && (strings.Contains(message, "no such file") || strings.Contains(message, "not found")) {
		return "container.shell-unavailable"
	}
	return ""
}

// endedShellReason names the reason of an exec that has ended. Docker reports a failed
// exec start inside the hijacked stream, then ends it with 126 or 127; only a verified
// missing /bin/sh names the reason, any other end stays unknown.
func (s *ContainerService) endedShellReason(ctx context.Context, containerID string, exitCode int) string {
	if (exitCode == 126 || exitCode == 127) && s.shellMissing(ctx, containerID) {
		return "container.shell-unavailable"
	}
	return ""
}

// shellMissing proves a missing /bin/sh in a container that still runs.
func (s *ContainerService) shellMissing(ctx context.Context, containerID string) bool {
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return false
	}
	if _, statErr := cli.ContainerStatPath(ctx, containerID, client.ContainerStatPathOptions{Path: "/bin/sh"}); !errdefs.IsNotFound(statErr) {
		return false
	}
	current, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	return err == nil && current.Container.State != nil && current.Container.State.Running
}
