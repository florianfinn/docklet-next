// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Transaction-taking compact storage primitives.

package activity

import (
	"errors"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// runtimeConflictInternal deliberately carries no receipt, key or bound identity.
// The handler owns permission checks and translation to a non-disclosing response.
type runtimeConflictInternal struct{}

func (*runtimeConflictInternal) Error() string { return "runtime request mismatch" }

var errRuntimeInvalidInternal = errors.New("invalid runtime storage input")

// An uninitialized store cannot establish key absence or executor continuity.
var errRuntimeGenerationUnavailableInternal = errors.New("runtime store generation unavailable")

// Every Tx helper requires the same caller-owned BEGIN IMMEDIATE transaction.
// Sample the existing service clock once, under runningMu before opening that
// transaction when composing with admission/worker state. Publish only after commit.
type runtimeIdentityInternal struct {
	RunID, StoreGeneration, ManagerID, RegistrationID string
	EnvironmentID, ExecutorGeneration, RequestDigest  string
}

type runtimeBindingInternal struct {
	Key, CallerID string
	CallerName    *string
}

type runtimeAdmissionInternal struct {
	Identity                          runtimeIdentityInternal
	Binding                           runtimeBindingInternal
	OperationID, TargetKind, TargetID string
	TargetName, ParentProjectID       *string
	GateScope, GateKey                string
}

func runtimeTimeInternal(now time.Time) time.Time { return now.UTC().Truncate(time.Microsecond) }

func runtimeUUIDInternal(value string) (string, error) {
	if len(value) != 36 {
		return "", errRuntimeInvalidInternal
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", errRuntimeInvalidInternal
	}
	return parsed.String(), nil
}

func runtimeGenerationTxInternal(tx *gorm.DB, now time.Time) (string, error) {
	var row RuntimeStoreGeneration
	err := tx.First(&row, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = RuntimeStoreGeneration{ID: 1, Generation: uuid.NewString(), CreatedAt: runtimeTimeInternal(now)}
		err = tx.Create(&row).Error
	}
	if err != nil {
		return "", err
	}
	parsed, err := uuid.Parse(row.Generation)
	if err != nil || parsed.Version() != 4 || parsed.Variant() != uuid.RFC4122 || parsed.String() != row.Generation {
		return "", errRuntimeInvalidInternal
	}
	return row.Generation, nil
}

func runtimeCheckStoreTxInternal(tx *gorm.DB, id runtimeIdentityInternal) error {
	if id.StoreGeneration == "" || id.ManagerID == "" || id.RegistrationID == "" ||
		id.EnvironmentID == "" || id.ExecutorGeneration == "" || id.RequestDigest == "" {
		return errRuntimeInvalidInternal
	}
	var own RuntimeStoreGeneration
	if err := tx.First(&own, "id = 1").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errRuntimeGenerationUnavailableInternal
		}
		return err
	}
	if own.Generation != id.StoreGeneration {
		return &runtimeConflictInternal{}
	}
	return nil
}

func runtimeMatchesInternal(row *RuntimeReceipt, id runtimeIdentityInternal) bool {
	return (id.RunID == "" || row.RunID == id.RunID) && row.StoreGeneration == id.StoreGeneration &&
		row.ManagerID == id.ManagerID && row.RegistrationID == id.RegistrationID &&
		row.EnvironmentID == id.EnvironmentID && row.ExecutorGeneration == id.ExecutorGeneration &&
		row.RequestDigest == id.RequestDigest
}

func runtimeReadTxInternal(tx *gorm.DB, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	if canonical, err := runtimeUUIDInternal(id.RunID); err != nil || canonical != id.RunID {
		return nil, errRuntimeInvalidInternal
	}
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	var row RuntimeReceipt
	if err := tx.First(&row, "run_id = ?", id.RunID).Error; err != nil {
		return nil, err
	}
	if !runtimeMatchesInternal(&row, id) {
		return nil, &runtimeConflictInternal{}
	}
	return &row, nil
}

// Absence is ErrRecordNotFound, never historical success or negative evidence.
// FirstSeenAt is audit only: a retained key has no server-side retry-age cutoff.
func runtimeLookupBindingTxInternal(tx *gorm.DB, id runtimeIdentityInternal, binding runtimeBindingInternal) (*RuntimeReceipt, error) {
	key, err := runtimeUUIDInternal(binding.Key)
	if err != nil || binding.CallerID == "" {
		return nil, errRuntimeInvalidInternal
	}
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	var bound RuntimeKeyBinding
	if err := tx.First(&bound, "environment_id = ? AND idempotency_key = ?", id.EnvironmentID, key).Error; err != nil {
		return nil, err
	}
	if bound.CallerID != binding.CallerID || bound.RequestDigest != id.RequestDigest {
		return nil, &runtimeConflictInternal{}
	}
	if id.RunID != "" && bound.RunID != id.RunID {
		return nil, &runtimeConflictInternal{}
	}
	id.RunID = bound.RunID
	row, err := runtimeReadTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "receipt" {
		return nil, &runtimeConflictInternal{}
	}
	return row, nil
}

func runtimeWriteErrorInternal(err error) error {
	if dbutil.IsUniqueConstraint(err) {
		return &runtimeConflictInternal{}
	}
	// Busy remains classifiable by the existing bounded caller retry policy.
	return err
}

// Hub intake persists the supplied raw UUID and client-key actor once. It does not arbitrate
// parent scopes, authorize, register workers, append output or dispatch effects.
// Callers must discard all returned evidence if their enclosing commit fails.
func runtimeAdmitTxInternal(tx *gorm.DB, now time.Time, request runtimeAdmissionInternal) (*RuntimeReceipt, error) {
	now = runtimeTimeInternal(now)
	id := request.Identity
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	canonical, err := runtimeUUIDInternal(id.RunID)
	if err != nil || canonical != id.RunID {
		return nil, errRuntimeInvalidInternal
	}
	lookupID := id
	lookupID.RunID = "" // A retry's newly proposed UUID cannot replace its original binding.
	bound, err := runtimeLookupBindingTxInternal(tx, lookupID, request.Binding)
	if err == nil {
		return bound, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	kind, ok := runtimeActivityTypeInternal(request.OperationID, request.TargetKind)
	if !ok || request.TargetID == "" || request.GateScope != request.TargetKind+":"+request.TargetID || request.GateKey == "" ||
		(request.ParentProjectID != nil && (request.TargetKind != "container" || *request.ParentProjectID == "")) {
		return nil, errRuntimeInvalidInternal
	}
	var existing RuntimeReceipt
	err = tx.First(&existing, "run_id = ?", id.RunID).Error
	if err == nil {
		// Reuse through a different key is an explicit join, never an admission.
		return nil, &runtimeConflictInternal{}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	row := RuntimeReceipt{
		RunID: id.RunID, Kind: "receipt", StoreGeneration: id.StoreGeneration, ManagerID: id.ManagerID,
		RegistrationID: id.RegistrationID, EnvironmentID: id.EnvironmentID,
		ExecutorGeneration: id.ExecutorGeneration, RequestDigest: id.RequestDigest,
		CallerID: new(request.Binding.CallerID), CallerName: request.Binding.CallerName,
		OperationID: new(request.OperationID), TargetKind: new(request.TargetKind), TargetID: new(request.TargetID),
		TargetName: request.TargetName, ParentProjectID: request.ParentProjectID,
		GateScope: new(request.GateScope), GateKey: new(request.GateKey), AdmittedAt: new(now),
		DispatchState: new("pending"), ExecutorState: new("pending"), FinalWriteAt: now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return nil, runtimeWriteErrorInternal(err)
	}
	model := Activity{
		BaseModel:     database.BaseModel{ID: id.RunID, CreatedAt: now, UpdatedAt: new(now)},
		EnvironmentID: id.EnvironmentID, Type: kind, Status: activitytypes.StatusRunning,
		ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName,
		StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedByDisplayName: row.CallerName,
		StartedAt: now, GateScope: request.GateScope, GateKey: request.GateKey,
		MessageCode: string(activitytypes.CodeRunStarted),
	}
	if err := tx.Create(&model).Error; err != nil {
		return nil, runtimeWriteErrorInternal(err)
	}
	if err := runtimeInsertBindingTxInternal(tx, now, &row, request.Binding); err != nil {
		return nil, err
	}
	return &row, nil
}

func runtimeActivityTypeInternal(operation, target string) (activitytypes.Type, bool) {
	var kind activitytypes.Type
	if target == "stack" {
		kind = map[string]activitytypes.Type{"deploy-project": activitytypes.TypeProjectDeploy,
			"down-project": activitytypes.TypeProjectDown, "restart-project": activitytypes.TypeProjectRestart,
			"redeploy-project": activitytypes.TypeProjectRedeploy, "update-project": activitytypes.TypeProjectUpdate, "restore-project": activitytypes.TypeProjectRestore}[operation]
	} else if target == "container" {
		kind = map[string]activitytypes.Type{"start-container": activitytypes.TypeContainerStart,
			"stop-container": activitytypes.TypeContainerStop, "restart-container": activitytypes.TypeContainerRestart}[operation]
	}
	return kind, kind != ""
}

func runtimeInsertBindingTxInternal(tx *gorm.DB, now time.Time, row *RuntimeReceipt, binding runtimeBindingInternal) error {
	key, err := runtimeUUIDInternal(binding.Key)
	if err != nil || binding.CallerID == "" || row.Kind != "receipt" {
		return errRuntimeInvalidInternal
	}
	return runtimeWriteErrorInternal(tx.Create(&RuntimeKeyBinding{EnvironmentID: row.EnvironmentID,
		IdempotencyKey: key, RunID: row.RunID, CallerID: binding.CallerID, CallerName: binding.CallerName,
		RequestDigest: row.RequestDigest, FirstSeenAt: runtimeTimeInternal(now)}).Error)
}

// The later handler must authorize this caller for the operation and activity
// before choosing a join. Existing keys always recheck their own caller/digest;
// there is no boolean authorization bypass and the original actor never changes.
func runtimeJoinTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal, binding runtimeBindingInternal) (*RuntimeReceipt, error) {
	bound, err := runtimeLookupBindingTxInternal(tx, id, binding)
	if err == nil {
		return bound, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	row, err := runtimeReadTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "receipt" {
		return nil, &runtimeConflictInternal{}
	}
	if err := runtimeInsertBindingTxInternal(tx, now, row, binding); err != nil {
		return nil, err
	}
	if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{}); err != nil {
		return nil, err
	}
	return row, nil
}

// Query absence commits a refusal fence with identity only. The caller must
// commit before answering not_received. Transport authentication and the hub's
// monotonic recent-negative-authority test are later F-c responsibilities.
func runtimeQueryTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	// Only the bound executor store may issue a refusal fence. A replacement
	// store cannot answer for a previously bound executor generation.
	if id.ExecutorGeneration != id.StoreGeneration {
		return nil, &runtimeConflictInternal{}
	}
	row, err := runtimeReadTxInternal(tx, id)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Missing generation is not permission to manufacture negative evidence.
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	now = runtimeTimeInternal(now)
	row = &RuntimeReceipt{RunID: id.RunID, Kind: "not_received", StoreGeneration: id.StoreGeneration,
		ManagerID: id.ManagerID, RegistrationID: id.RegistrationID, EnvironmentID: id.EnvironmentID,
		ExecutorGeneration: id.ExecutorGeneration, RequestDigest: id.RequestDigest,
		SettledAt: new(now), FinalWriteAt: now}
	if err := tx.Create(row).Error; err != nil {
		return nil, runtimeWriteErrorInternal(err)
	}
	return row, nil
}

type runtimeEvidenceInternal struct {
	State                string
	UnknownReason        *string
	ResultCode, Error    *string
	StartedAt, SettledAt *time.Time
}

func runtimeEqualInternal[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func runtimeExecutorTimeInternal(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return new(runtimeTimeInternal(*value))
}

// Applies authenticated executor evidence only, never hub disposition or an
// admission tuple. Compose with the conditional activity/final-message writer in
// this same tx later; terminal executor evidence itself cannot be rewritten.
func runtimeEvidenceTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal, evidence runtimeEvidenceInternal) (*RuntimeReceipt, error) {
	row, err := runtimeReadTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "receipt" {
		return nil, &runtimeConflictInternal{}
	}
	terminal := evidence.State == "success" || evidence.State == "failed" || evidence.State == "cancelled"
	if (!terminal && evidence.State != "accepted" && evidence.State != "executing" && evidence.State != "unknown") ||
		terminal != (evidence.SettledAt != nil) || (evidence.State == "unknown") != (evidence.UnknownReason != nil) ||
		(evidence.UnknownReason != nil && *evidence.UnknownReason == "") ||
		(!terminal && (evidence.ResultCode != nil || evidence.Error != nil)) {
		return nil, errRuntimeInvalidInternal
	}
	evidence.StartedAt = runtimeExecutorTimeInternal(evidence.StartedAt)
	evidence.SettledAt = runtimeExecutorTimeInternal(evidence.SettledAt)
	same := *row.ExecutorState == evidence.State && runtimeEqualInternal(row.UnknownReason, evidence.UnknownReason) &&
		runtimeEqualInternal(row.ExecutorResultCode, evidence.ResultCode) && runtimeEqualInternal(row.ExecutorError, evidence.Error) &&
		runtimeEqualInternal(row.ExecutorStartedAt, evidence.StartedAt) && runtimeEqualInternal(row.ExecutorSettledAt, evidence.SettledAt)
	if same {
		return row, nil
	}
	if row.ExecutorSettledAt != nil || (*row.ExecutorState == "executing" && evidence.State == "accepted") ||
		(row.ExecutorStartedAt != nil && !runtimeEqualInternal(row.ExecutorStartedAt, evidence.StartedAt)) {
		return nil, &runtimeConflictInternal{}
	}
	updates := map[string]any{"executor_state": evidence.State, "unknown_reason": evidence.UnknownReason,
		"executor_result_code": evidence.ResultCode, "executor_error": evidence.Error,
		"executor_started_at": evidence.StartedAt, "executor_settled_at": evidence.SettledAt}
	if terminal && row.SettledAt == nil {
		updates["settled_at"] = runtimeTimeInternal(now)
	}
	if err := runtimeUpdateTxInternal(tx, now, row, updates); err != nil {
		return nil, err
	}
	return row, nil
}

type runtimeDispositionInternal struct {
	Disposition, By, Reason string
	ReleaseAcknowledged     bool
}

// Trusted hub storage only: the later caller owns administrator authorization,
// explicit risk acknowledgment and stopping/joining dispatch before release.
// This independent immutable tuple survives all subsequent executor evidence.
func runtimeDispositionTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal, disposition runtimeDispositionInternal) (*RuntimeReceipt, error) {
	row, err := runtimeReadTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "receipt" {
		return nil, &runtimeConflictInternal{}
	}
	switch disposition.Disposition {
	case "cancelled", "interrupted", "released", "refused", "not_received":
	default:
		return nil, errRuntimeInvalidInternal
	}
	if strings.TrimSpace(disposition.By) == "" || strings.TrimSpace(disposition.Reason) == "" || disposition.ReleaseAcknowledged != (disposition.Disposition == "released") {
		return nil, errRuntimeInvalidInternal
	}
	if row.HubDisposition != nil {
		if *row.HubDisposition == disposition.Disposition && *row.HubDispositionBy == disposition.By &&
			*row.HubDispositionReason == disposition.Reason && row.ReleaseAcknowledged == disposition.ReleaseAcknowledged {
			return row, nil
		}
		return nil, &runtimeConflictInternal{}
	}
	updates := map[string]any{"hub_disposition": disposition.Disposition, "hub_disposition_by": disposition.By,
		"hub_disposition_reason": disposition.Reason, "release_acknowledged": disposition.ReleaseAcknowledged,
		"hub_disposition_at": runtimeTimeInternal(now)}
	if row.SettledAt == nil {
		updates["settled_at"] = runtimeTimeInternal(now)
	}
	if err := runtimeUpdateTxInternal(tx, now, row, updates); err != nil {
		return nil, err
	}
	return row, nil
}

func runtimeUpdateTxInternal(tx *gorm.DB, now time.Time, row *RuntimeReceipt, updates map[string]any) error {
	// Backward jumps may delay cleanup; they cannot shorten a previously earned floor.
	now = runtimeTimeInternal(now)
	if now.Before(row.FinalWriteAt) {
		now = row.FinalWriteAt
	}
	updates["final_write_at"] = now
	if err := tx.Model(&RuntimeReceipt{}).Where("run_id = ?", row.RunID).Updates(updates).Error; err != nil {
		return runtimeWriteErrorInternal(err)
	}
	if err := tx.First(row, "run_id = ?", row.RunID).Error; err != nil {
		return err
	}
	if row.OperationID != nil && runtimeImageOperation(RuntimeOperation(*row.OperationID)) && row.SettledAt != nil {
		return tx.Table("image_lifecycle_runs").Where("id = ?", row.RunID).Update("settled", true).Error
	}
	return nil
}
