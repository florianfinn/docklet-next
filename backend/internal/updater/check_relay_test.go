// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proof of the relayed unsupported-layout check refusal.

package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

// The agent writes the OperationError itself as the problem body; the hub relays only
// the unsupported-layout code, never credential or other remote failures.
func TestRemoteCheckRelaysOnlyTheLayoutRefusal(t *testing.T) {
	body := func(code string) []byte {
		raw, err := json.Marshal(imageDTO.Failure(code, 409))
		require.NoError(t, err)
		return raw
	}
	refusal := relayedCheckRefusal(fmt.Errorf("proxy: %w", &remenv.StatusError{StatusCode: 409, Body: body("update.image-binding-unsupported")}))
	var problem *imageDTO.OperationError
	require.True(t, errors.As(refusal, &problem))
	require.Equal(t, "update.image-binding-unsupported", problem.Problem.Code)
	require.Equal(t, 409, problem.GetStatus())
	for _, err := range []error{
		nil,
		errors.New("transport"),
		&remenv.StatusError{StatusCode: 403, Body: body("update.image-binding-unsupported")},
		&remenv.StatusError{StatusCode: 500, Body: body("update.image-binding-unsupported")},
		&remenv.StatusError{StatusCode: 409, Body: body("update.config-drift")},
		&remenv.StatusError{StatusCode: 409, Body: []byte("not json")},
	} {
		require.Nil(t, relayedCheckRefusal(err), "%v", err)
	}
}
