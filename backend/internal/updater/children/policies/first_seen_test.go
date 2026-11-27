// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package policies

import (
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestFirstSeenRestartIdentityRetentionAndCopy(t *testing.T) {
	f := newStoreFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	c := dto.UpdateCandidate{ImageRef: "registry.example.com/app:latest", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}
	require.NoError(t, f.ObserveFirstSeen(t.Context(), c, now))
	f.reopen(t)
	source := []dto.UpdateCandidate{c}
	out, e := f.AgeCandidates(t.Context(), source, 2, now.Add(time.Hour))
	require.NoError(t, e)
	require.Equal(t, now.Add(48*time.Hour), *out[0].EligibleAt)
	require.Nil(t, source[0].EligibleAt)
	c.ImageRef = "registry.example.com/app:other"
	require.NoError(t, f.ObserveFirstSeen(t.Context(), c, now.Add(time.Hour)))
	out, e = f.AgeCandidates(t.Context(), []dto.UpdateCandidate{c}, 1, now.Add(25*time.Hour))
	require.NoError(t, e)
	require.Nil(t, out[0].EligibleAt)
	prior := c
	c.Manifest = "sha256:" + strings.Repeat("b", 64)
	require.NoError(t, f.ObserveFirstSeen(t.Context(), c, now.Add(25*time.Hour)))
	out, e = f.AgeCandidates(t.Context(), []dto.UpdateCandidate{prior, c}, 1, now.Add(25*time.Hour))
	require.NoError(t, e)
	require.Nil(t, out[0].EligibleAt)
	require.NotNil(t, out[1].EligibleAt)
	require.NoError(t, f.ObserveFirstSeen(t.Context(), c, now.Add(122*24*time.Hour)))
	var rows []FirstSeen
	require.NoError(t, f.DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, c.Manifest, rows[0].Manifest)
	require.Equal(t, now.Add(122*24*time.Hour), rows[0].FirstSeenAt)
}
