// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original image config build time plausibility contract.

package image

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildTimeKeepsOnlyPlausibleBuilds(t *testing.T) {
	require.Nil(t, BuildTime(time.Time{}), "an absent created field")
	require.Nil(t, BuildTime(time.Unix(0, 0)), "reproducible builds stamp the epoch")
	require.Nil(t, BuildTimeText(""))
	require.Nil(t, BuildTimeText("not a time"))
	require.Nil(t, BuildTimeText("1970-01-01T00:00:00Z"))
	built := BuildTimeText("2026-11-24T07:00:00+01:00")
	require.NotNil(t, built)
	require.Equal(t, "2026-11-24T06:00:00Z", built.Format(time.RFC3339), "build times are stored in UTC")
}
