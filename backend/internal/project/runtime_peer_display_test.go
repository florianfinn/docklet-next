// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original display-name contract for issue 531 peer blockers. M34 #547: duplicates block only name-addressed lookups.

package project

import (
	"strings"
	"testing"
	"unicode/utf8"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

// Stack names are free text; every name that reaches a blocker or a display
// page must survive the transport allowlists.
func TestS4UnsafeDuplicateNameKeepsBlocker(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	target := runtimeIdentityProject(t, svc, root, "example-target", "target", "name: shared_stack\n"+identityServices)
	runtimeIdentityProject(t, svc, root, "example-peer", "peer", "name: shared_stack\n"+identityServices)
	require.NoError(t, svc.db.Model(target).Update("name", "Media: Plex").Error)
	_, found, _, err := svc.ResolveRuntimeProjectIdentityAcknowledged(t.Context(), "", "shared_stack", "")
	require.False(t, found)
	b := s4Problem(t, err)
	require.Equal(t, "compose.duplicate", b.Reason)
	require.Equal(t, "Media Plex", b.Name)
	require.False(t, b.Confirmable)
}

func TestRuntimeDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"media":       "media",
		"Media: Plex": "Media Plex",
		"web/api\\x":  "web api x",
		"::":          "example-id",
		"tab\there":   "tab here",
		"":            "example-id",
	} {
		require.Equal(t, want, imageDTO.RuntimeDisplayName(in, "example-id"), in)
	}
	long := imageDTO.RuntimeDisplayName("a:"+strings.Repeat("ä", 100), "example-id")
	require.LessOrEqual(t, len(long), 128)
	require.True(t, utf8.ValidString(long))
}
