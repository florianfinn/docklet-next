// Source: docklet-next/issues/439
// Revision: 7564d315b6fbfd50e17265f52af0e2c70c450af7
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original helper file-ownership and pre-effect restore proofs (M21 R1-S8-01, R1-S8-03).

package selfupdate

import (
	"context"
	"errors"
	"os"
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

// The helper runs as root, so its own new files must get the original owners:
// the result file the application identity, which differs from the helper.
func TestHelperGivesItsOwnFilesTheOriginalOwners(t *testing.T) {
	data, request, _ := requestFixture(t)
	request.AppUID, request.AppGID = os.Geteuid()+7, os.Getegid()+7
	require.NoError(t, WriteJSON(RequestPath(data, request.OpID), request))
	calls := recordChown(t, nil)
	helper := Helper{Up: func(context.Context, *composetypes.Project, string) error { return nil }, Healthy: func(context.Context, Request) error { return nil }}
	require.NoError(t, helper.Run(t.Context(), data, request.OpID))
	require.NotEmpty(t, *calls)
	for _, call := range *calls {
		require.Equal(t, []int{request.AppUID, request.AppGID}, []int{call.uid, call.gid}, call.name)
	}
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "healthy", result.Phase)
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
