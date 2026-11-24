// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original retry generation and credential fencing proofs.

package tailnet_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	dto "github.com/florianfinn/docklet-next/types/tailnet"
	"github.com/stretchr/testify/require"
)

func rejectedManager(t *testing.T) (*tailnet.Manager, *tailnettest.Fleet) {
	t.Helper()
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(errors.New("backend: requested tags [tag:docklet-hub] are invalid or not permitted"))
	m := newManager(t, fleet, func(c *tailnet.ManagerConfig) { c.JoinTimeout = 150 * time.Millisecond })
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return m.Connection().Code == dto.CodeTagsNotPermitted }, time.Second, time.Millisecond)
	return m, fleet
}

func TestRetryJoinImmediateAndSetupSnapshot(t *testing.T) {
	m, fleet := rejectedManager(t)
	revision := m.Status().CredentialRevision
	gen := m.Status().Generation
	result, err := m.Test(t.Context())
	require.NoError(t, err)
	require.False(t, result.Stale)
	require.Equal(t, revision, result.CredentialRevision)
	require.Equal(t, revision, m.Status().CredentialRevision)
	require.Equal(t, gen+1, m.Status().Generation)
	require.EqualValues(t, 2, fleet.Created.Load())
	require.Equal(t, "pass", result.Checks[3].State)
	require.True(t, m.RetryJoin(t.Context(), revision))
	require.EqualValues(t, 2, fleet.Created.Load(), "connected hub is never restarted")
	require.False(t, m.RetryJoin(t.Context(), "different-revision"))
	require.EqualValues(t, 2, fleet.Created.Load())
}

func TestRetryJoinFencesConcurrentSaveAndReset(t *testing.T) {
	for _, change := range []string{"save", "same-credential-save", "reset"} {
		t.Run(change, func(t *testing.T) {
			m, fleet := rejectedManager(t)
			release := fleet.HoldUps()
			defer release()
			result := make(chan dto.TestResult, 1)
			go func() { out, err := m.Test(t.Context()); require.NoError(t, err); result <- out }()
			require.Eventually(t, func() bool { return fleet.Created.Load() == 2 }, time.Second, time.Millisecond)
			select {
			case <-result:
				t.Fatal("test completed before the new join was observed")
			default:
			}
			if change == "reset" {
				require.NoError(t, m.Reset(t.Context()))
			} else {
				next := manual
				if change == "save" {
					next.AuthKey = tailnettest.Key("auth", "replacement")
				}
				m.Apply(t.Context(), true, next)
			}
			out := <-result
			require.True(t, out.Stale)
			for _, check := range out.Checks[3:] {
				require.Equal(t, "skipped", check.State)
			}
		})
	}
}

func TestRetryJoinTimeoutAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[cancel], func(t *testing.T) {
			m, fleet := rejectedManager(t)
			release := fleet.HoldUps()
			defer release()
			ctx, stop := context.WithCancel(t.Context())
			defer stop()
			done := make(chan bool, 1)
			started := time.Now()
			go func() { done <- m.RetryJoin(ctx, m.Status().CredentialRevision) }()
			require.Eventually(t, func() bool { return fleet.Created.Load() == 2 }, time.Second, time.Millisecond)
			select {
			case <-done:
				t.Fatal("retry completed without a terminal observation")
			default:
			}
			if cancel {
				stop()
			}
			require.True(t, <-done)
			if !cancel {
				require.GreaterOrEqual(t, time.Since(started), 140*time.Millisecond)
			}
			require.Equal(t, "joining", m.Connection().State)
		})
	}
}

func TestRetryJoinReturnsSuperseded(t *testing.T) {
	for _, change := range []string{"save", "reset", "close"} {
		t.Run(change, func(t *testing.T) {
			m, fleet := rejectedManager(t)
			release := fleet.HoldUps()
			defer release()
			revision := m.Status().CredentialRevision
			done := make(chan bool, 1)
			go func() { done <- m.RetryJoin(t.Context(), revision) }()
			require.Eventually(t, func() bool { return fleet.Created.Load() == 2 }, time.Second, time.Millisecond)
			select {
			case <-done:
				t.Fatal("retry completed while join held")
			default:
			}
			switch change {
			case "save":
				m.Apply(t.Context(), true, manual)
			case "reset":
				require.NoError(t, m.Reset(t.Context()))
			case "close":
				m.Close(t.Context())
			}
			require.False(t, <-done, "a superseded generation cannot complete the retry")
		})
	}
}

func TestJoinTagRefusalDoesNotLogProviderText(t *testing.T) {
	logs := captureLogs(t)
	fleet := tailnettest.NewFleet()
	secret := tailnettest.Key("client", "example-secret")
	fleet.FailNextUps(errors.New("backend: requested tags [tag:docklet-hub " + secret + "] are invalid or not permitted"))
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return m.Connection().Code == dto.CodeTagsNotPermitted }, time.Second, time.Millisecond)
	require.Equal(t, []string{tailnet.HubTag}, m.Connection().RefusedTags)
	require.NotContains(t, logs.String(), secret)
	require.NotContains(t, logs.String(), "requested tags")
}

func TestJoinFailureDoesNotLogRawProviderText(t *testing.T) {
	logs := captureLogs(t)
	fleet := tailnettest.NewFleet()
	fleet.FailNextUps(errors.New("provider-private-example: no response"))
	m := newManager(t, fleet)
	m.Apply(t.Context(), true, manual)
	require.Eventually(t, func() bool { return m.Connection().State == "connected" }, time.Second, time.Millisecond)
	require.Contains(t, logs.String(), "Tailnet join failed; retrying")
	require.NotContains(t, logs.String(), "provider-private-example")
}
