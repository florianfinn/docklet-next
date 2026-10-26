// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Stack-only update and restore intent; snapshots never cross this boundary.

package updater

import (
	"context"
	"time"
)

// Complete restore evidence must fit the same frozen intent limits as admission.
const MaxLifecycleServices = 256
const MaxLifecycleIntentBytes = 128 << 10

type LifecycleRequest struct {
	PointID              string `json:"pointId,omitempty"`
	ExpectedRevision     string `json:"expectedRevision" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$"`
	ReplaceConfiguration bool   `json:"replaceConfiguration,omitempty"`
}

// LifecycleIntent is produced by executor preflight, then hashed into runtime
// admission. It contains identities only, never Compose/environment contents.
type LifecycleIntent struct {
	Policy           *PolicyBinding   `json:"policy,omitempty"`
	Request          LifecycleRequest `json:"request"`
	TargetIdentity   string           `json:"targetIdentity"`
	SavedRevision    string           `json:"savedRevision"`
	ObservedRevision string           `json:"observedRevision"`
	BaselineRevision string           `json:"baselineRevision"`
	PointRevision    string           `json:"pointRevision"`
	Selections       []ImageSelection `json:"selections" maxItems:"256"`
}

type ImageSelection struct {
	Service         string `json:"service"`
	SourceReference string `json:"sourceReference"`
	Reference       string `json:"reference"`
	Digest          string `json:"digest"`
	ImageID         string `json:"imageId"`
	Platform        string `json:"platform"`
	Kind            string `json:"kind" enum:"version,rebuild,restore"`
}

type PointImage struct {
	Service   string `json:"service"`
	Reference string `json:"reference"`
	ImageID   string `json:"imageId"`
	Digest    string `json:"digest,omitempty"`
	Present   bool   `json:"present"`
	Running   bool   `json:"running"`
}
type RestorePoint struct {
	ProtectionReason   *RestorePointProtectionReason `json:"protectionReason" nullable:"true" enum:"restore-target,healthy-baseline"`
	HoldState          RestorePointHoldState         `json:"holdState" enum:"none,active,unknown"`
	FailedAttempt      bool                          `json:"failedAttempt"`
	Images             []PointImage                  `json:"images"`
	ID                 string                        `json:"id"`
	RegistrationID     string                        `json:"registrationId"`
	TargetID           string                        `json:"targetId"`
	TargetIdentity     string                        `json:"targetIdentity"`
	SourceRunID        string                        `json:"sourceRunId"`
	CreatedAt          time.Time                     `json:"createdAt"`
	SourceState        string                        `json:"sourceState" enum:"settled,partial-source"`
	VolumeDataIncluded bool                          `json:"volumeDataIncluded"`
	Available          bool                          `json:"available"`
	Orphaned           bool                          `json:"orphaned"`
	Code               string                        `json:"code,omitempty"`
}

type BaselineStatus struct {
	TargetIdentity   string `json:"targetIdentity"`
	AutoExcluded     bool   `json:"autoExcluded"`
	SavedRevision    string `json:"savedRevision"`
	ObservedRevision string `json:"observedRevision"`
	Available        bool   `json:"available"`
	Code             string `json:"code,omitempty"`
	SourceRunID      string `json:"sourceRunId,omitempty"`
}

// ExecutionIdentity is installed only by the trusted runtime adapter.
type ExecutionIdentity struct{ RunID, RegistrationID string }
type executionKey struct{}

func WithExecutionIdentity(ctx context.Context, id ExecutionIdentity) context.Context {
	return context.WithValue(ctx, executionKey{}, id)
}
func ExecutionIdentityFromContext(ctx context.Context) ExecutionIdentity {
	v, _ := ctx.Value(executionKey{}).(ExecutionIdentity)
	return v
}

// LifecycleResult is bounded public run evidence; no snapshots or daemon output.
type LifecycleResult struct {
	HTTPStatus         int    `json:"httpStatus,omitempty"`
	RunID              string `json:"runId"`
	RestorePointID     string `json:"restorePointId,omitempty"`
	Code               string `json:"code,omitempty"`
	BaselineCode       string `json:"baselineCode,omitempty"`
	Verified           bool   `json:"verified"`
	VolumeDataIncluded bool   `json:"volumeDataIncluded"`
}

// UncertainImageEffect retains runtime exclusion for inspected tag disposition;
// returning from a worker is not proof that an external writer can be undone.
type UncertainImageEffect struct{ Cause error }

func (e *UncertainImageEffect) Error() string          { return "runtime.outcome-unknown" }
func (e *UncertainImageEffect) RuntimeUncertain() bool { return true }

// ResultSink is an application-owned handoff to runtime settlement. A worker
// reports only bounded public evidence; the receipt transaction persists it.
type resultSinkKey struct{}

func WithResultSink(ctx context.Context, sink func(LifecycleResult)) context.Context {
	return context.WithValue(ctx, resultSinkKey{}, sink)
}
func RecordLifecycleResult(ctx context.Context, result LifecycleResult) {
	if sink, ok := ctx.Value(resultSinkKey{}).(func(LifecycleResult)); ok {
		sink(result)
	}
}
