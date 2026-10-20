// Source: docklet-next/docs/design/run-gate.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Real process lifetime-lock, crash release and descriptor-close proof.

package database

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunGate_HubLockProcessFixture(t *testing.T) {
	path := os.Getenv("RUN_GATE_LOCK_PATH")
	if path == "" {
		return
	}
	if os.Getenv("RUN_GATE_CLOSE_RETAIN_LOCK") == "1" {
		db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
		require.NoError(t, err)
		pool, err := db.SQLDB()
		require.NoError(t, err)
		require.NoError(t, db.CloseRetainingHubLock())
		require.Error(t, pool.Ping())
		runtime.GC()
		fmt.Println("READY")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}
	lock, err := acquireHubLock(t.Context(), path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer lock.Stop()
	// Closing an unrelated descriptor must preserve the descriptor-owned lock.
	other, err := os.Open(path + ".lock")
	require.NoError(t, err)
	require.NoError(t, other.Close())
	fmt.Println("READY")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

type matrixLockProcess struct {
	cmd    *exec.Cmd
	stdin  *os.File
	cancel context.CancelFunc
	stderr matrixLockOutput
}

// os/exec can still copy stderr when the parent reads startup diagnostics.
type matrixLockOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *matrixLockOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *matrixLockOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func matrixStartLockProcess(t *testing.T, path string, extraEnv ...string) *matrixLockProcess {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	p := &matrixLockProcess{cancel: cancel}
	p.cmd = exec.CommandContext(ctx, exe, "-test.run=^TestRunGate_HubLockProcessFixture$")
	p.cmd.Env = append(append(os.Environ(), "RUN_GATE_LOCK_PATH="+path), extraEnv...)
	stdout, err := p.cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := p.cmd.StdinPipe()
	require.NoError(t, err)
	p.stdin = stdin.(*os.File)
	p.cmd.Stderr = &p.stderr
	require.NoError(t, p.cmd.Start())
	t.Cleanup(func() {
		cancel()
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, p.stderr.String())
	require.Equal(t, "READY", strings.TrimSpace(line))
	return p
}
func TestRunGate_Case31_SecondHubFailsFast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	p := matrixStartLockProcess(t, path)
	start := time.Now()
	_, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.ErrorIs(t, err, ErrHubAlreadyRunning)
	require.Less(t, time.Since(start), time.Second)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "refused hub must not open or migrate either database")
	_, err = p.stdin.Write([]byte("stop\n"))
	require.NoError(t, err)
	require.NoError(t, p.cmd.Wait(), p.stderr.String())
	db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Close())
}
func TestRunGate_Case34_CrashReleasesStableLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	p := matrixStartLockProcess(t, path)
	require.NoError(t, p.cmd.Process.Kill())
	require.Error(t, p.cmd.Wait())
	lock, err := acquireHubLock(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, lock.Stop())
	_, err = os.Stat(path + ".lock")
	require.NoError(t, err)
}
func TestHubLockDescriptorClosePreservesChildRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	lock, err := acquireHubLock(t.Context(), path)
	require.NoError(t, err)
	defer lock.Stop()
	other, err := os.Open(path + ".lock")
	require.NoError(t, err)
	require.NoError(t, other.Close())
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestRunGate_HubLockProcessFixture$")
	cmd.Env = append(os.Environ(), "RUN_GATE_LOCK_PATH="+path)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "hub.already_running")
}

func TestFailedShutdownRetainsLockUntilProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	p := matrixStartLockProcess(t, path, "RUN_GATE_CLOSE_RETAIN_LOCK=1")
	_, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.ErrorIs(t, err, ErrHubAlreadyRunning)
	_, err = p.stdin.Write([]byte("stop\n"))
	require.NoError(t, err)
	require.NoError(t, p.cmd.Wait(), p.stderr.String())
	db, err := InitializeHub(t.Context(), sqlitestore.URL(path), MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Close())
}
