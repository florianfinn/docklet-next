// Source: getarcaneapp/arcane/backend/pkg/utils/iconcatalog/iconcatalog_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package iconcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve_SingleVariantFallsBackWithinLevel(t *testing.T) {
	resolved := Resolve(CatalogSelfhst, IconSet{Light: "postgres"})
	require.Equal(t, "https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/postgres-light.svg", resolved.IconLightURL)
	require.Equal(t, "https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/postgres-dark.svg", resolved.IconDarkURL)
}
