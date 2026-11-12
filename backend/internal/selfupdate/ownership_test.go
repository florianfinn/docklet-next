// Source: docklet-next/issues/439
// Revision: 7564d315b6fbfd50e17265f52af0e2c70c450af7
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original helper file-ownership and pre-effect restore proofs (M21 R1-S8-01, R1-S8-03, V-S8-02).

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/require"
)

type chownCall struct {
	name     string
	uid, gid int
}

func recordChown(t *testing.T, fail error) *[]chownCall {
	t.Helper()
	calls := &[]chownCall{}
	previous := chownFile
	chownFile = func(f *os.File, uid, gid int) error {
		*calls = append(*calls, chownCall{f.Name(), uid, gid})
		return fail
	}
	t.Cleanup(func() { chownFile = previous })
	return calls
}

// operatorEnv makes the env file appear owned by an operator identity that
// differs from both the helper and the application, with mode 0600.
func operatorEnv(t *testing.T, request Request) (uid, gid int) {
	t.Helper()
	require.NoError(t, os.Chmod(request.Replay.EnvFile, 0600))
	uid, gid = os.Geteuid()+3, os.Getegid()+3
	previous := envOwner
	envOwner = func(info os.FileInfo) (*syscall.Stat_t, bool) {
		return &syscall.Stat_t{Uid: uint32(uid), Gid: uint32(gid)}, true
	}
	t.Cleanup(func() { envOwner = previous })
	return uid, gid
}

// The helper runs as root, so its own new files must get the original owners:
// env replacement and backup the env owner, the result file the application.
func TestHelperGivesItsOwnFilesTheOriginalOwners(t *testing.T) {
	data, request, _ := requestFixture(t)
	request.AppUID, request.AppGID = os.Geteuid()+7, os.Getegid()+7
	require.NoError(t, WriteJSON(RequestPath(data, request.OpID), request))
	envUID, envGID := operatorEnv(t, request)
	calls := recordChown(t, nil)
	helper := Helper{Up: func(context.Context, *composetypes.Project, string) error { return nil }, Healthy: func(context.Context, Request) error { return nil }}
	require.NoError(t, helper.Run(t.Context(), data, request.OpID))
	envDir, backup := filepath.Dir(request.Replay.EnvFile), request.Replay.EnvFile+".docklet-previous."+request.OpID
	seen := map[string]int{}
	for _, call := range *calls {
		switch {
		case call.name == backup:
			seen["backup"]++
			require.Equal(t, []int{envUID, envGID}, []int{call.uid, call.gid}, call.name)
		case filepath.Dir(call.name) == envDir:
			seen["env"]++
			require.Equal(t, []int{envUID, envGID}, []int{call.uid, call.gid}, call.name)
		case filepath.Dir(call.name) == filepath.Dir(ResultPath(data, request.OpID)):
			seen["result"]++
			require.Equal(t, []int{request.AppUID, request.AppGID}, []int{call.uid, call.gid}, call.name)
		default:
			t.Fatalf("unrelated file chowned: %s", call.name)
		}
	}
	require.Equal(t, []bool{true, true, true}, []bool{seen["backup"] == 1, seen["env"] == 1, seen["result"] > 0}, seen)
	for _, path := range []string{request.Replay.EnvFile, backup} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm(), path)
	}
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "healthy", result.Phase)
}

// An env owner the helper cannot reproduce ends the operation before any
// replacement: original bytes, no backup left behind and no Compose up.
func TestUnreproducibleEnvOwnerEndsPreEffect(t *testing.T) {
	data, request, old := requestFixture(t)
	envUID, _ := operatorEnv(t, request)
	previous := chownFile
	chownFile = func(f *os.File, uid, gid int) error {
		if uid == envUID {
			return errors.New("operation not permitted")
		}
		return previous(f, uid, gid)
	}
	t.Cleanup(func() { chownFile = previous })
	ups := 0
	helper := Helper{Up: func(context.Context, *composetypes.Project, string) error { ups++; return nil }, Healthy: func(context.Context, Request) error { return nil }}
	require.ErrorContains(t, helper.Run(t.Context(), data, request.OpID), CodeLayout)
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	_, backupErr := os.Lstat(request.Replay.EnvFile + ".docklet-previous." + request.OpID)
	require.Equal(t, []any{"failed-pre-effect", true, true, 0}, []any{result.Phase, envBytesEqual(request.Replay.EnvFile, old), os.IsNotExist(backupErr), ups})
}

// A file whose owner cannot be set is refused before any replacement.
func TestOwnLikeRefusesWhenTheOwnerCannotBeSet(t *testing.T) {
	recordChown(t, errors.New("operation not permitted"))
	f, err := os.CreateTemp(t.TempDir(), "owned-*")
	require.NoError(t, err)
	defer f.Close()
	err = ownLike(f, &syscall.Stat_t{Uid: uint32(os.Geteuid() + 7), Gid: uint32(os.Getegid() + 7)})
	require.ErrorContains(t, err, CodeLayout)
}

// A refused env replacement leaves the original bytes, so the operation ends
// failed-pre-effect at once instead of waiting for the deadline.
func TestRefusedEnvReplacementEndsPreEffect(t *testing.T) {
	data, request, old := requestFixture(t)
	previous := replaceEnv
	replaceEnv = func(string, []byte, os.FileMode, *syscall.Stat_t) error { return Refuse(CodeLayout) }
	t.Cleanup(func() { replaceEnv = previous })
	ups := 0
	helper := Helper{Up: func(context.Context, *composetypes.Project, string) error { ups++; return nil }, Healthy: func(context.Context, Request) error { return nil }}
	require.Error(t, helper.Run(t.Context(), data, request.OpID))
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "failed-pre-effect", result.Phase)
	require.True(t, envBytesEqual(request.Replay.EnvFile, old))
	require.Zero(t, ups)
}
