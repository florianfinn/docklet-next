// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity

import (
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/updater"
)

const RuntimePeerContractVersion = 20
const RuntimeHubContractHeader = "X-Docklet-Hub-Contract"

type RuntimePeerAdmissionRequest struct {
	AcknowledgedUncertainty string `json:"acknowledgedUncertainty,omitempty" pattern:"^[a-fA-F0-9]{64}$"`
}

type RuntimeDeployAdmissionRequest struct {
	project.DeployOptions
	RuntimePeerAdmissionRequest
}

type RuntimeLifecycleAdmissionRequest struct {
	updater.LifecycleRequest
	RuntimePeerAdmissionRequest
}
