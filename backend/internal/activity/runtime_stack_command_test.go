// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original distinct stop digest and stack-scoped service restart admission proof.
package activity

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"sync"
	"testing"
)

func TestStopProjectOwnDigestNoOptionsAndReceipt(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	command.Operation = RuntimeStopProject
	command.Options = RuntimeOptions{}
	stop, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	down := command
	down.Operation = RuntimeDownProject
	canonicalDown, err := runtimeCanonicalizeInternal(down)
	require.NoError(t, err)
	require.NotEqual(t, stop.digest, canonicalDown.digest)
	require.NotEqual(t, stop.gateKey, canonicalDown.gateKey)
	command.Options.RestartServices = []string{}
	_, err = runtimeCanonicalizeInternal(command)
	require.Error(t, err)
	command = stop.command
	admitted, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	row := b3Receipt(t, f, admitted.ActivityID)
	require.Equal(t, new(string(RuntimeStopProject)), row.OperationID)
}
func TestServiceRestartKeysAndAdmissionRemainStackScoped(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	f := b2Fixture(t, t.Context(), func(_ context.Context, command RuntimeCommand, _ io.Writer) error {
		close(entered)
		<-release
		return nil
	})
	command := f.command()
	command.Operation = RuntimeRestartProject
	command.Options = RuntimeOptions{RestartServices: []string{"app"}}
	one, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	other := command
	other.Key = uuid.NewString()
	other.Options.RestartServices = []string{"worker"}
	two, err := runtimeCanonicalizeInternal(other)
	require.NoError(t, err)
	require.NotEqual(t, one.gateKey, two.gateKey)
	require.Equal(t, one.gateScope, two.gateScope)
	admitted, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	b1Await(t, entered)
	_, err = f.service.Admit(t.Context(), other)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	unblock()
	_, err = b2Wait(t, f, command, admitted.ActivityID)
	require.NoError(t, err)
}
