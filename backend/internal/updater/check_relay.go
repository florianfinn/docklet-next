// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original relay of the agent's typed unsupported-layout check refusal.

package updater

import (
	"encoding/json"
	"errors"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

// relayedCheckRefusal passes an agent's unsupported-layout refusal through to the
// hub caller; every other remote failure keeps its existing mapping.
func relayedCheckRefusal(err error) error {
	var status *remenv.StatusError
	if !errors.As(err, &status) || status.StatusCode < 400 || status.StatusCode >= 500 || remenv.AgentAuthRejected(status.StatusCode) {
		return nil
	}
	var body imageDTO.OperationError
	if json.Unmarshal(status.Body, &body) != nil || body.Success || body.Problem.Code != "update.image-binding-unsupported" {
		return nil
	}
	return imageDTO.Failure(body.Problem.Code, status.StatusCode)
}
