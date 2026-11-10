// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The binary differs only by the private helper data path and role build tag.
// This permits the real CLI/process boundary without a Docker daemon or mounts.
func TestHelperProcessBoundaryAndInterruption(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		t.Run(role, func(t *testing.T) { runHelperProcess(t, role) })
	}
}
func runHelperProcess(t *testing.T, role string) {
	data, request, old := requestFixture(t)
	socket := "@docklet-selfupdate-" + uuid.NewString()
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	entered := make(chan struct{}, 2)
	var calls atomic.Int32
	receiver := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	receiver.Listener = listener
	receiver.Start()
	defer receiver.Close()
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "docklet-"+role)
	tags := ""
	if role == "agent" {
		tags = "exclude_frontend"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-p=2", "-tags="+tags, "-ldflags=-X github.com/florianfinn/docklet-next/backend/internal/selfupdate.HelperData="+data, "-o", binary, "./backend/cmd")
	build.Dir = root
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "self-update", "--op", request.OpID)
	command.Env = []string{"DOCKER_HOST=unix://" + socket, "HUB_IMAGE=must-not-win", "INPUT=must-not-win"}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	require.NoError(t, command.Start())
	select {
	case <-entered:
	case <-ctx.Done():
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("real helper did not reach Engine: %s", stderr.String())
	}
	var phase Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &phase))
	require.Equal(t, "up-started", phase.Phase)
	require.NoError(t, command.Process.Kill())
	require.Error(t, command.Wait())
	require.False(t, envBytesEqual(request.Replay.EnvFile, old))
	backup, err := os.ReadFile(request.Replay.EnvFile + ".docklet-previous." + request.OpID)
	require.NoError(t, err)
	require.Equal(t, old, backup)
	retry := exec.CommandContext(ctx, binary, "self-update", "--op", request.OpID)
	retry.Env = command.Env
	result, err := retry.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(result), CodeUnknown)
	require.EqualValues(t, 1, calls.Load(), "interrupted operation must not resend Engine work")
	store := openStore(t, filepath.Join(t.TempDir(), "operation.db"))
	defer closeStore(t, store)
	op := operationFixture()
	op.OpID = request.OpID
	op.TargetRef = request.TargetReference
	_, _, err = store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	observer := Observer{Store: store, Data: data, Version: func() string { return "1.0.0" }, Running: func(context.Context, Operation) (bool, error) { return false, nil }}
	done, err := observer.Advance(t.Context())
	require.NoError(t, err)
	require.True(t, done)
	settled, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, CodeUnknown, settled.Code)
	final, err := os.ReadFile(request.Replay.EnvFile + ".docklet-previous." + request.OpID)
	require.NoError(t, err)
	require.Equal(t, old, final)
}
