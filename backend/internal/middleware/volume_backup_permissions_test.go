// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original backup-upload permission parity before Direct/Edge forwarding.

package middleware

import (
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestProxiedVolumeRestoreUploadRequiresBothPermissions(t *testing.T) {
	matcher := authz.NewPermissionMatcher()
	matcher.Add(http.MethodPost, "/volumes/{volumeName}/backups/upload", authz.PermVolumesUpload)
	m := newProxyAuthzMiddleware(matcher)
	for _, permissions := range [][]string{nil, {authz.PermVolumesUpload}, {authz.PermVolumesBackup}, {authz.PermVolumesUpload, authz.PermVolumesBackup}} {
		ps := authz.NewPermissionSet()
		ps.AddEnv(proxyTestEnvID, permissions...)
		c := newProxyRequestContext(http.MethodPost, "/api/environments/"+proxyTestEnvID+"/volumes/data/backups/upload")
		require.Equal(t, len(permissions) != 2, m.proxyPermissionDenied(c, ps, proxyTestEnvID), permissions)
	}
}
