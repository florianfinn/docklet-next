// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell frame protocol, Docker resize and joined process cleanup.

package container

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/types/container"
	"github.com/moby/moby/client"
)

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
	reason := "container.shell-unavailable"
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
		return
	}
	session, err := s.AttachExec(ctx, containerID, execID)
	if err != nil {
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
			if session.Close(cleanup) != nil {
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
		reason = "container.shell-unavailable"
	case err := <-inputDone:
		inputEnded = true
		if err != nil && !wshub.IsExpectedClose(err) {
			reason = "container.shell-protocol"
		}
	case err := <-outputDone:
		outputEnded = true
		if err != nil {
			reason = "container.shell-unavailable"
		}
	}
	closing.Store(true)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	inspected, inspectErr := session.Inspect(cleanupCtx)
	if outputEnded && inspectErr == nil && !inspected.Running {
		code = &inspected.ExitCode
		session.hijackedResp.Close()
	} else {
		if err := session.Close(cleanupCtx); err != nil {
			reason = "container.shell-orphaned"
		}
		if inspected, err := session.Inspect(cleanupCtx); err == nil && !inspected.Running {
			code = &inspected.ExitCode
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
			if _, err := session.Stdin().Write(data); err != nil {
				return err
			}
			continue
		}
		var resize container.ShellResize
		if kind != websocket.MessageText || json.Unmarshal(data, &resize, json.RejectUnknownMembers(true)) != nil || resize.Type != "resize" || resize.Cols < 1 || resize.Cols > 1000 || resize.Rows < 1 || resize.Rows > 1000 {
			return errors.New("container.shell-protocol")
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
