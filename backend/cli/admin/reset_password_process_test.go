//go:build linux

// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Actual offline recovery command with a terminal, child process and ownership assertions.

package admin

import (
	"bytes"
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPasswordResetProcessFixture(t *testing.T) {
	if os.Getenv("ADMIN_RECOVERY_CHILD") != "1" {
		return
	}
	resetPasswordUsername = os.Getenv("ADMIN_RECOVERY_USER")
	resetPasswordCmd.SetContext(t.Context())
	resetPasswordCmd.SetOut(os.Stdout)
	if err := runResetPasswordCommandInternal(resetPasswordCmd, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}
func TestPasswordResetRefusesLiveHubAndWorksOffline(t *testing.T) {
	dir := t.TempDir()
	url := sqlitestore.URL(filepath.Join(dir, "hub.db"))
	db, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	target, _ := createGlobalAdminInternal(t, db)
	username := target.Username
	id := target.ID
	exe, err := os.Executable()
	require.NoError(t, err)
	child := func() *exec.Cmd {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPasswordResetProcessFixture$")
		cmd.Env = append(os.Environ(), "ADMIN_RECOVERY_CHILD=1", "ADMIN_RECOVERY_USER="+username, "DATABASE_URL="+url, "DOCKER_CONFIG="+filepath.Join(dir, ".docker"), "ALLOW_CLI_PASSWORD_RESET=true", "PUID="+strconv.Itoa(os.Geteuid()), "PGID="+strconv.Itoa(os.Getegid()))
		return cmd
	}
	live := child()
	output, err := live.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "hub.already_running")
	require.NoError(t, db.Close())
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	require.NoError(t, err)
	master := os.NewFile(uintptr(masterFD), "master")
	defer master.Close()
	require.NoError(t, unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0))
	number, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	require.NoError(t, err)
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0600)
	require.NoError(t, err)
	cmd := child()
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	require.NoError(t, cmd.Start())
	require.NoError(t, slave.Close())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	chunks := make(chan string, 16)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 1024)
		for {
			n, err := master.Read(buffer)
			if n > 0 {
				chunks <- string(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	var text bytes.Buffer
	prompt := func(want string) {
		t.Helper()
		for !strings.Contains(text.String(), want) {
			select {
			case chunk, ok := <-chunks:
				require.True(t, ok, "terminal closed before prompt")
				text.WriteString(chunk)
			case <-time.After(5 * time.Second):
				t.Fatal("password recovery did not reach terminal prompt")
			}
		}
	}
	prompt("New password: ")
	_, err = io.WriteString(master, "New-Synthetic-Passphrase-42!\n")
	require.NoError(t, err)
	prompt("Confirm new password: ")
	_, err = io.WriteString(master, "New-Synthetic-Passphrase-42!\n")
	require.NoError(t, err)
	require.NoError(t, cmd.Wait())
	for range chunks {
	}
	reopened, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	defer reopened.Close()
	svc := user.NewUserService(reopened, nil, session.RevokeAllUserSessionsExceptInDB)
	updated, err := svc.GetUserByID(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, svc.ValidatePassword(updated.PasswordHash, "New-Synthetic-Passphrase-42!"))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		stat := info.Sys().(*syscall.Stat_t)
		require.EqualValues(t, os.Geteuid(), stat.Uid, entry.Name())
		require.EqualValues(t, os.Getegid(), stat.Gid, entry.Name())
	}
}
