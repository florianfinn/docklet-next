// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded agent send regressions.

package autoheal

import (
	"context"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/autoheal"
	nDTO "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
)

func TestAgentProjectionIsBoundedBehindABlockedTransport(t *testing.T) {
	s, _ := healStore(t)
	s.Config.AgentMode = true
	s.DispatchWait = 50 * time.Millisecond
	release, sent := make(chan struct{}), make(chan string, 1)
	// The send ignores its context, like the Edge tunnel send, until released or after 3 s.
	s.Dispatch = func(_ context.Context, p nDTO.DispatchRequest) error {
		select {
		case <-release:
		case <-time.After(3 * time.Second):
		}
		sent <- p.AutoHeal.EventID
		return nil
	}
	r, _, err := s.Reserve(t.Context(), "run", "container", "web", "app", Baseline{}, dto.Settings{MaxRestarts: 5, WindowMinutes: 30})
	require.NoError(t, err)
	require.NoError(t, s.Settle(t.Context(), r.ID, "restart-failed"))
	started := time.Now()
	require.NoError(t, s.Project(t.Context(), r.ID, true))
	require.Less(t, time.Since(started), 2*time.Second, "a blocked transport must not hold the Auto Heal run")
	close(release)
	select {
	case id := <-sent:
		require.NotEmpty(t, id)
	case <-time.After(2 * time.Second):
		t.Fatal("the determined outcome was never sent")
	}
}
