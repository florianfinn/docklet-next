// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSettingsValidation(t *testing.T) {
	good := Update{MaxRestarts: 5, WindowMinutes: 37, ExcludedStacks: []string{"app"}, ExcludedContainers: []string{"web.1"}}
	require.NoError(t, good.Validate())
	for _, mutate := range []func(*Update){func(u *Update) { u.MaxRestarts = 0 }, func(u *Update) { u.MaxRestarts = 11 }, func(u *Update) { u.WindowMinutes = 4 }, func(u *Update) { u.WindowMinutes = 1441 }, func(u *Update) { u.ExcludedStacks = []string{"UPPER"} }, func(u *Update) { u.ExcludedContainers = []string{"/web"} }} {
		u := good
		mutate(&u)
		require.Error(t, u.Validate())
	}
}
