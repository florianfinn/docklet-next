// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original foreign-file restore-offer boundary proof.

package traefik

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP30ForeignInvalidFileHasDiagnosticWithoutRestoreOffer(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", "http: [")
	result, err := x.readResult()
	require.NoError(t, err)
	require.Equal(t, dto.InvalidGraph, result.Eligibility.Reason)
	require.Equal(t, "dynamic/routes.yml", result.Eligibility.Path)
	require.Positive(t, result.Eligibility.Line)
	require.NotEmpty(t, result.Eligibility.Detail)
	require.Len(t, result.Files, 1)
	require.False(t, result.Files[0].Owned)
	require.Empty(t, result.Files[0].LastVerifiedHash, "a foreign file must not offer last-good restore")
	require.Equal(t, "http: [", x.read("dynamic/routes.yml"))
	key := "host\x00" + x.i.Source.Canonical
	verified := proxy.Snapshot([]byte(tiny), true)
	require.NoError(t, x.db.Exec("INSERT INTO proxy_verified_files(source,path,hash,content) VALUES(?,?,?,?)", key, "dynamic/routes.yml", verified.Hash, verified.Content).Error)
	result, err = x.readResult()
	require.NoError(t, err)
	require.True(t, result.Files[0].Owned)
	require.Equal(t, verified.Hash, result.Files[0].LastVerifiedHash)
}
