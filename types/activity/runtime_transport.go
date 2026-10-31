// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Transport carriers.

package activity

import (
	"github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/updater"
)

// RuntimeOperation selects only an existing lifecycle operation, never a route
// or arbitrary command. Receivers must validate the operation/target/options pair.
type RuntimeOperation string

const (
	RuntimeUpdateProject     RuntimeOperation = "update-project"
	RuntimeRestoreProject    RuntimeOperation = "restore-project"
	RuntimeDeployProject     RuntimeOperation = "deploy-project"
	RuntimeDownProject       RuntimeOperation = "down-project"
	RuntimeStopProject       RuntimeOperation = "stop-project"
	RuntimeRecreateContainer RuntimeOperation = "recreate-container"
	RuntimeRedeployContainer RuntimeOperation = "redeploy-container"
	RuntimeRestartProject    RuntimeOperation = "restart-project"
	RuntimeRedeployProject   RuntimeOperation = "redeploy-project"
	RuntimeStartContainer    RuntimeOperation = "start-container"
	RuntimeStopContainer     RuntimeOperation = "stop-container"
	RuntimeRestartContainer  RuntimeOperation = "restart-container"
)

// RuntimeTarget contains a canonical raw ID, not a browser host~ID or alias.
// ParentProjectID is explicit null for a stack or an unparented container; a
// non-null parent is the persisted project ID. Name is outside the semantic digest; stack execution rechecks the receiver-local resolved Compose name.
type RuntimeTarget struct {
	Kind            string  `json:"kind"`
	ID              string  `json:"id"`
	ParentProjectID *string `json:"parentProjectId"`
	Name            *string `json:"name,omitempty"`
}

// RuntimeOptions reuses all four legacy deploy fields. RestartServices is a
// sorted unique set; nil/empty means whole stack for restart only. Keep null
// versus [] on the wire so an unsupported supplied option is not hidden.
type RuntimeOptions struct {
	Recreate        *container.Edit          `json:"recreate,omitzero"`
	Lifecycle       *updater.LifecycleIntent `json:"lifecycle,omitempty"`
	Deploy          *project.DeployOptions   `json:"deploy"`
	RestartServices []string                 `json:"restartServices"`
}

// RuntimeExecutorIdentity is bound by authenticated construction, never granted
// by JSON. ExecutorGeneration names executor storage, not the hub's local store.
// Authenticated metadata can expose this tuple before binding a dispatch.
type RuntimeExecutorIdentity struct {
	ManagerID          string `json:"managerId"`
	RegistrationID     string `json:"registrationId"`
	ExecutorGeneration string `json:"executorGeneration"`
}

// RuntimeRunIdentity is the execute/query/cancel reference. The hub assigns the
// same UUID used by the executor. Its local database environment is always 0;
// the hub records its registered host separately. No hub database ID travels.
type RuntimeRunIdentity struct {
	RuntimeExecutorIdentity
	RunID         string `json:"runId"`
	RequestDigest string `json:"requestDigest"`
}

// RuntimeAttribution is original bounded audit attribution, never authorization.
// ID is nonempty; ID/username/display are valid UTF-8, at most 256 bytes and
// contain no controls. It contains no preferences, roles, grants or credentials.
type RuntimeAttribution struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"displayName,omitempty"`
}

// RuntimeExecutionEnvelope carries no client key or clock-based authority.
// The receiver must recompute the canonical semantic digest and match Identity,
// bind authenticated ownership, then commit acceptance before starting work.
// These carriers alone enforce neither HTTP size limits nor unknown-field rules.
type RuntimeExecutionEnvelope struct {
	Identity    RuntimeRunIdentity `json:"identity"`
	Operation   RuntimeOperation   `json:"operation" enum:"deploy-project,down-project,restart-project,redeploy-project,start-container,stop-container,restart-container,update-project,restore-project,stop-project,recreate-container,redeploy-container"`
	Target      RuntimeTarget      `json:"target"`
	Options     RuntimeOptions     `json:"options"`
	Attribution RuntimeAttribution `json:"attribution"`
}

type RuntimeAnswerState string

const (
	RuntimeAnswerActive      RuntimeAnswerState = "active"
	RuntimeAnswerTerminal    RuntimeAnswerState = "terminal"
	RuntimeAnswerNotReceived RuntimeAnswerState = "not-received"
)

// RuntimeExecutionOutcome is a finite snapshot acknowledgment/query/cancel
// answer, never an execution stream. Active/terminal require Detail with the
// same run ID and executor environment 0; terminal means settled workflow only.
// NotReceived requires a committed matching tombstone and no Detail. Missing,
// offline or unreadable evidence is an error, never a not-received answer.
type RuntimeExecutionOutcome struct {
	Identity RuntimeRunIdentity      `json:"identity"`
	State    RuntimeAnswerState      `json:"state"`
	Detail   *RuntimeExecutionDetail `json:"detail,omitempty"`
}

// RuntimeExecutionDetail preserves existing activity/message/step vocabulary.
// Activity times are executor times, never cross-host ordering evidence. Metadata
// and message payloads are observations, never command or permission inputs.
// Producers and consumers enforce the bounded window, validation and unseen-line relay.
type RuntimeExecutionDetail struct {
	Activity         RuntimeSnapshot              `json:"activity"`
	Messages         []Message                    `json:"messages"`
	StepTrace        []StepTraceEntry             `json:"stepTrace"`
	Output           []RuntimeExecutionOutputLine `json:"output"`
	OutputTruncated  bool                         `json:"outputTruncated"`
	RetainedStartSeq uint64                       `json:"retainedStartSeq"`
}

// RuntimeExecutionOutputLine leaves legacy OutputLine JSON unchanged. OutputSeq
// is positive, executor-local and monotonic for this run, unrelated to log WS seq
// or timestamps. RetainedStartSeq is the first retained seq (next seq if empty);
// eviction is explicit and retention remains the existing 256 KiB bounded window.
type RuntimeExecutionOutputLine struct {
	OutputLine
	OutputSeq uint64 `json:"outputSeq"`
}
