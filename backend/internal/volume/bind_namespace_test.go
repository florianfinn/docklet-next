// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindHelperAndLockNamespace(t *testing.T) {
	count := 0
	removed := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/images/"):
			_, _ = w.Write([]byte(`{"Id":"tools"}`))
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			count++
			if count == 1 {
				_, _ = w.Write([]byte(`{"Id":"named-helper"}`))
			} else {
				_, _ = w.Write([]byte(fmt.Sprintf(`{"Id":"bind-helper-%d"}`, count)))
			}
		case r.Method == "DELETE":
			removed = r.URL.Path
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected helper effect %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	svc := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), helperByVolume: map[string]*volumeHelper{}}
	key := volumetypes.BindKey("/srv/app")
	named, err := svc.resolveHelperInternal(t.Context(), newVolumeServiceTestDockerClientInternal(t, server), "app")
	require.NoError(t, err)
	bound, err := svc.resolveHelperInternal(context.WithValue(t.Context(), bindLayoutKey{}, "directory"), newVolumeServiceTestDockerClientInternal(t, server), key)
	require.NoError(t, err)
	require.NotEqual(t, named, bound)
	require.Len(t, svc.helperByVolume, 2)
	require.NotNil(t, svc.helperByVolume[key], "literal bind identity owns the registry entry")
	require.Equal(t, bound, svc.helperByVolume[key].id)
	namedUnlock, ok := svc.workspaceLocks.TryLock("app")
	require.True(t, ok)
	defer namedUnlock()
	bindUnlock, ok := svc.workspaceLocks.TryLock(key)
	require.True(t, ok)
	defer bindUnlock()
	_, ok = svc.workspaceLocks.TryLock(key)
	require.False(t, ok)
	changed, err := svc.resolveHelperInternal(context.WithValue(t.Context(), bindLayoutKey{}, "file"), newVolumeServiceTestDockerClientInternal(t, server), key)
	require.NoError(t, err)
	require.NotEqual(t, bound, changed)
	require.Contains(t, removed, bound)
	require.Equal(t, "file", svc.helperByVolume[key].kind)
}

func TestRenameRefusesPaddedBindKeys(t *testing.T) {
	key := volumetypes.BindKey("/srv/app")
	for _, names := range [][2]string{{" " + key + " ", "renamed"}, {"data", key + "\n"}} {
		_, err := (&VolumeService{}).RenameVolume(t.Context(), names[0], names[1], usertypes.Actor{})
		require.EqualError(t, err, "volume.bind.unsupported")
	}
}
