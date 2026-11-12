// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"encoding/hex"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestBindKeyValidation(t *testing.T) {
	for _, source := range []string{"/srv/app", "/srv/app.conf", "/srv/ü", "/" + strings.Repeat("a", 254)} {
		key := BindKey(source)
		decoded, err := BindSource(key)
		require.NoError(t, err)
		require.Equal(t, source, decoded)
		require.Equal(t, source, DisplayName(key))
	}
	invalid := []string{"bind:2F737276", "bind:gg", "bind:f", "bind:ff"}
	for _, source := range []string{"", "/", "srv/app", "/a/../b", "/a/", "//a", "/a\x00b", "/" + strings.Repeat("a", 255)} {
		invalid = append(invalid, "bind:"+hex.EncodeToString([]byte(source)))
	}
	for _, key := range invalid {
		t.Run(key, func(t *testing.T) {
			_, err := BindSource(key)
			require.Error(t, err)
			require.EqualError(t, err, "volume.bind.invalid-key")
		})
	}
	require.NoError(t, ValidateName("app"))
	require.EqualError(t, RefuseBind(BindKey("/srv/app")), "volume.bind.unsupported")
}
