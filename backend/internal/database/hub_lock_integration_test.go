//go:build postgres_integration

// Source: florianfinn/docklet-next/docs/design/run-gate.md
// Revision: ab701984e86075694baabdc5efe14bed287768fc
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: PostgreSQL process tests for the single-hub startup lock.

package database

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestRunGate_HubLockProcessFixture(t *testing.T) {
	if os.Getenv("RUN_GATE_LOCK_CHILD") == "" {
		t.Skip("subprocess fixture")
	}
	pool, err := sql.Open("pgx", os.Getenv("RUN_GATE_LOCK_DSN"))
	require.NoError(t, err)
	defer pool.Close()
	lock, err := acquireHubLock(t.Context(), pool)
	require.NoError(t, err)
	fmt.Println("READY")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	require.NoError(t, lock.Stop())
}

type matrixLockProcess struct {
	cmd    *exec.Cmd
	stdin  *os.File
	cancel context.CancelFunc
	stderr bytes.Buffer
}

func matrixStartLockProcess(t *testing.T, dsn string) *matrixLockProcess {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	p := &matrixLockProcess{cancel: cancel}
	p.cmd = exec.CommandContext(ctx, exe, "-test.run=^TestRunGate_HubLockProcessFixture$")
	p.cmd.Env = append(os.Environ(), "RUN_GATE_LOCK_CHILD=hold", "RUN_GATE_LOCK_DSN="+dsn)
	stdout, err := p.cmd.StdoutPipe()
	require.NoError(t, err)
	stdin, err := p.cmd.StdinPipe()
	require.NoError(t, err)
	p.cmd.Stderr = &p.stderr
	require.NoError(t, p.cmd.Start())
	readPipe, ok := stdin.(*os.File)
	require.True(t, ok)
	p.stdin = readPipe
	t.Cleanup(func() {
		p.cancel()
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

func matrixLockDatabase(t *testing.T) string {
	t.Helper()
	sourceDSN := os.Getenv("DOCKLET_TEST_POSTGRES_DSN")
	require.NotEmpty(t, sourceDSN)
	source, err := url.Parse(sourceDSN)
	require.NoError(t, err)
	admin, err := sql.Open("pgx", sourceDSN)
	require.NoError(t, err)
	var suffix [8]byte
	_, err = rand.Read(suffix[:])
	require.NoError(t, err)
	name := "docklet_hub_lock_" + hex.EncodeToString(suffix[:])
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, dropErr := admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		require.NoError(t, dropErr)
		require.NoError(t, admin.Close())
	})
	source.Path = "/" + name
	return source.String()
}

func matrixLockPool(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	pool, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	return pool
}

func TestRunGate_Case31_SecondHubFailsFast(t *testing.T) {
	dsn := matrixLockDatabase(t)
	p := matrixStartLockProcess(t, dsn)
	start := time.Now()
	_, err := InitializeHub(t.Context(), dsn, MigrationOptions{})
	require.ErrorIs(t, err, ErrHubAlreadyRunning)
	require.Less(t, time.Since(start), time.Second)
	_, err = p.stdin.Write([]byte("stop\n"))
	require.NoError(t, err)
	require.NoError(t, p.cmd.Wait(), p.stderr.String())
}

func TestRunGate_Case34_HeldOldSessionRejectsNewHub(t *testing.T) {
	dsn := matrixLockDatabase(t)
	p := matrixStartLockProcess(t, dsn)
	pool := matrixLockPool(t, dsn)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := acquireHubLock(ctx, pool)
	require.ErrorIs(t, err, ErrHubAlreadyRunning)
	require.Less(t, time.Since(start), 500*time.Millisecond)
	_, err = p.stdin.Write([]byte("stop\n"))
	require.NoError(t, err)
	require.NoError(t, p.cmd.Wait(), p.stderr.String())
}

func TestHubLockStopClosesPhysicalSession(t *testing.T) {
	dsn := matrixLockDatabase(t)
	firstPool := matrixLockPool(t, dsn)
	secondPool := matrixLockPool(t, dsn)
	lock, err := acquireHubLock(t.Context(), firstPool)
	require.NoError(t, err)
	require.NoError(t, lock.Stop())
	replacement, err := acquireHubLock(t.Context(), secondPool)
	require.NoError(t, err)
	require.NoError(t, replacement.Stop())
}
