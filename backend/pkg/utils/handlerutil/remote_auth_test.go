// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Agent authentication failures remain distinct from caller authorization for #229.

package handlerutil

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClosingRemoteAuthTranslation(t *testing.T) {
	for _, status := range []int{401, 403} {
		err := TranslateRemoteProxyError(&remenv.StatusError{StatusCode: status, Body: []byte("synthetic-private")})
		var typed huma.StatusError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, 502, typed.GetStatus())
		require.NotContains(t, err.Error(), "synthetic-private")
	}
}

func TestClosingRemoteStatusTranslation(t *testing.T) {
	for _, status := range []int{0, 200, 307, 999} {
		err := TranslateRemoteProxyError(&remenv.StatusError{StatusCode: status, Body: []byte("synthetic-private")})
		var typed huma.StatusError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, 502, typed.GetStatus())
		require.NotContains(t, err.Error(), "synthetic-private")
	}
	err := TranslateRemoteProxyError(&remenv.StatusError{StatusCode: 409})
	var typed huma.StatusError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, 409, typed.GetStatus())
}
