// Source: docklet-next/issues/320
// Revision: 561c228593dd679affe6c577ac95942573032590
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original guard that the hub-local origin overlay never writes Docker list maps.

package container

import (
	"reflect"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	containertypes "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func overlayService() *ContainerService {
	return &ContainerService{operations: &containerOperations{origins: &origin.Store{LocalRegistration: "local"}}}
}

func sharedListResult(labels map[string]string) ContainerListResult {
	item := containertypes.Summary{ID: "example-id", Names: []string{"/app"}, Labels: labels}
	item.Origin = containertypes.OriginUnknown
	return ContainerListResult{Items: []containertypes.Summary{item}}
}

func TestOriginOverlayLeavesSharedLabelMapsUntouched(t *testing.T) {
	labels := map[string]string{"net.unraid.docker.managed": "dockerman"}
	result := sharedListResult(labels)
	require.NoError(t, overlayLocal(t.Context(), overlayService(), &result))
	require.Equal(t, containertypes.OriginUnraid, result.Items[0].Origin, "the overlay result still reaches the caller")
	require.False(t, reflect.ValueOf(labels).UnsafePointer() == reflect.ValueOf(result.Items[0].Labels).UnsafePointer(),
		"the overlaid result must not alias the coalesced Docker list map")
	require.Equal(t, map[string]string{"net.unraid.docker.managed": "dockerman"}, labels)
}

// Coalesced list reads hand the same label maps to concurrent requests.
func TestConcurrentOriginOverlaysOverSharedLabelMaps(t *testing.T) {
	labels := map[string]string{"app": "example", "tier": "web"}
	service := overlayService()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				result := sharedListResult(labels)
				if err := overlayLocal(t.Context(), service, &result); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
