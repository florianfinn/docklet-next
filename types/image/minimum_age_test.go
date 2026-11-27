// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package image

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMinimumAgeReason(t *testing.T) { require.NoError(t, ValidateReason("update.minimum-age")) }
