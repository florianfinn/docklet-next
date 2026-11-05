// Source: docklet-next/issues/341
// Revision: 01f87a4c35d7a054ec346b16c1ba62fd5581b27a
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A manual check that cannot reach its environment reports the typed unavailable code.

package updater

import (
	"context"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

func TestManualCheckTransportFailureIsUnavailable(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "check")
	f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
		return nil, fmt.Errorf("failed to send request to environment host: %w", &remenv.TransportError{Err: context.DeadlineExceeded})
	}
	_, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	var failure *imageDTO.OperationError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "update.check-unavailable", failure.Problem.Code)
	require.Equal(t, 503, failure.Status)
}
