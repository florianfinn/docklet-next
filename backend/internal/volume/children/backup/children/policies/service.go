// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; single owning-host policy, target selection and scheduler-produced next-run metadata; M12 backup foundation; stable codes instead of diagnostics on policy failures.

package policies

import (
	"context"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	backupdto "github.com/florianfinn/docklet-next/types/backup"
	"log/slog"
	"strings"
	"sync"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/entityjobs"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
)

const defaultSchedule = "0 0 2 * * *"

// Dependencies are the backup operations scheduled policies run through.
type Dependencies struct {
	ValidateTarget func(context.Context, string) error
	DB             *database.DB
	Location       *time.Location
	Activity       *activity.ActivityService
	AlreadyRunning error
	LatestRun      func(ctx context.Context, policyID string) (*volume.BackupEntry, error)
	CreateBackup   func(ctx context.Context, volumeName, policyID string) (*volume.Backup, error)
	Reconcile      func(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error)
}

// Service owns per-volume backup policies and their scheduled jobs.
type Service struct {
	deps      Dependencies
	jobs      *entityjobs.Registry
	admission sync.Mutex
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps, jobs: entityjobs.New("volume-backup:", backup.VolumeAdmissionScope)}
}

// SetScheduler injects the dynamic scheduler and admission gate for per-policy
// backup jobs. Agent mode passes them too: agents run their own volume backups.
func (s *Service) SetScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.jobs.SetScheduler(ctx, dynamicScheduler, admissionGate)
}

func (s *Service) policiesInternal(ctx context.Context, volumeName string) ([]VolumeBackupPolicy, error) {
	var policies []VolumeBackupPolicy
	if err := s.deps.DB.WithContext(ctx).Where("volume_name = ?", volumeName).Order("created_at ASC").Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("failed to load volume backup policies: %w", err)
	}
	return policies,
		nil
}

// Policy loads the volume's current policy when policyID is empty.
func (s *Service) Policy(ctx context.Context, volumeName, policyID string) (*VolumeBackupPolicy, error) {
	var policy VolumeBackupPolicy
	query := s.deps.DB.WithContext(ctx).Where("volume_name = ?", volumeName)
	if strings.TrimSpace(policyID) != "" {
		query = query.Where("id = ?", policyID)
	}
	err := query.First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load volume backup policy: %w", err)
	}
	return &policy, nil
}

func (s *Service) GetBackupPolicies(ctx context.Context, volumeName string) (*volume.BackupPolicyCollection, error) {
	policies, err := s.policiesInternal(ctx, volumeName)
	if err != nil {
		return nil, err
	}
	location := s.deps.Location
	if location == nil {
		location = time.UTC
	}
	result := &volume.BackupPolicyCollection{TimeZone: location.String(), Policies: make([]volume.BackupPolicy, 0, len(policies))}
	for i := range policies {
		lastRun, runErr := s.deps.LatestRun(ctx, policies[i].ID)
		if runErr != nil {
			return nil, fmt.Errorf("failed to load latest volume backup: %w", runErr)
		}
		dto := policies[i].ToDTO(lastRun)
		if scheduler, ok := s.jobs.Scheduler().(interface {
			GetJobRuntimeState(string) (scheduler.JobRuntimeState, bool)
		}); ok {
			if state, found := scheduler.GetJobRuntimeState(s.jobs.JobName(policies[i].ID)); found && dto.Enabled {
				dto.NextRunAt = state.NextRun
			}
		}
		result.Policies = append(result.Policies, dto)
	}
	return result, nil
}

func (s *Service) UpdateBackupPolicies(ctx context.Context, volumeName string, updates []volume.UpdateBackupPolicy) (*volume.BackupPolicyCollection, error) {
	return s.UpdateBackupPoliciesChecked(ctx, volumeName, updates, nil)
}

// UpdateBackupPoliciesChecked validates the received target set while holding
// the same admission mutex as target sync and reference collection.
func (s *Service) UpdateBackupPoliciesChecked(ctx context.Context, volumeName string, updates []volume.UpdateBackupPolicy, check func() error) (*volume.BackupPolicyCollection, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	if check != nil {
		if err := check(); err != nil {
			return nil, err
		}
	}
	if len(updates) > 1 {
		return nil, backupdto.NewProblem(422, "volume.backup.policy-single", nil)
	}

	existing, err := s.policiesInternal(ctx, volumeName)
	if err != nil {
		return nil, err
	}
	for _, update := range updates {
		if s.deps.ValidateTarget != nil {
			if err := s.deps.ValidateTarget(ctx, update.TargetID); err != nil {
				return nil, err
			}
		}
		if update.ID != "" && (len(existing) == 0 || update.ID != existing[0].ID) {
			return nil, backupdto.NewProblem(422, "volume.backup.policy-invalid", nil)
		}
		if _, e := backup.ValidatePolicyUpdate(ctx, "volume", update); e != nil {
			return nil, backupdto.NewProblem(422, "volume.backup.policy-invalid", e)
		}
	}
	reconcile := backup.PolicyReconciliation[VolumeBackupPolicy, volume.UpdateBackupPolicy]{
		Domain:   "volume",
		DB:       s.deps.DB,
		Existing: existing,
		ID:       func(policy *VolumeBackupPolicy) string { return policy.ID },
		UpdateID: func(update volume.UpdateBackupPolicy) string { return update.ID },
		New:      func() VolumeBackupPolicy { return VolumeBackupPolicy{VolumeName: volumeName} },
		Build: func(ctx context.Context, policy *VolumeBackupPolicy, update volume.UpdateBackupPolicy) error {
			normalized, validatePolicyUpdateErr := backup.ValidatePolicyUpdate(ctx, "volume", update)
			if validatePolicyUpdateErr != nil {
				return validatePolicyUpdateErr
			}
			update = normalized
			policy.Enabled, policy.Schedule, policy.RetentionCount = update.Enabled, update.Schedule, update.RetentionCount
			policy.StopContainers, policy.LocalEnabled = update.StopContainers, update.LocalEnabled
			policy.TargetID = update.TargetID
			if policy.TargetID == "" {
				policy.TargetID = "local"
			}
			return nil
		},
		Unregister: s.jobs.Unregister,
		Reschedule: s.rescheduleInternal,
	}
	if runErr := reconcile.Run(ctx, updates); runErr != nil {
		return nil, runErr
	}
	return s.GetBackupPolicies(ctx, volumeName)
}

func (s *Service) runScheduledBackupInternal(ctx context.Context, policyID string) (scheduler.Outcome, error) {
	var policy VolumeBackupPolicy
	if err := s.deps.DB.WithContext(ctx).Where("id = ? AND enabled = ?", policyID, true).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return scheduler.Outcome{Status: scheduler.Canceled, Message: "Backup policy disabled or deleted"}, nil
		}
		return scheduler.Outcome{}, publicPolicyErrorInternal(err, 500)
	}
	if previous, ok := jobcontext.Run(ctx); ok {
		outcome := jobcontext.ConfirmedTarget(previous, policy.VolumeName)
		if outcome.Status == scheduler.Succeeded {
			return outcome, nil
		}
	}
	var entry *volume.Backup
	activityID, err := activitylib.RunHandlerActivity(ctx, s.deps.Activity, activitylib.HandlerOptions{
		EnvironmentID:  "0",
		Type:           activitytypes.TypeResourceAction,
		ResourceType:   "volume_backup",
		ResourceID:     policy.VolumeName,
		ResourceName:   policy.VolumeName,
		User:           &user.SystemUser,
		Step:           "Creating scheduled backup",
		Message:        "Creating scheduled volume backup",
		SuccessMessage: "Scheduled volume backup created successfully",
		Metadata: database.JSON{
			"action":         "scheduled_volume_backup",
			"policyId":       policy.ID,
			"schedule":       policy.Schedule,
			"volumeName":     policy.VolumeName,
			"retentionCount": policy.RetentionCount,
			"stopContainers": policy.StopContainers,
			"localEnabled":   policy.LocalEnabled,
		},
	}, func(activityCtx context.Context) error {
		var backupErr error
		entry, backupErr = s.deps.CreateBackup(activityCtx, policy.VolumeName, policy.ID)
		return backupErr
	})
	if errors.Is(err, s.deps.AlreadyRunning) {
		slog.InfoContext(ctx, "Scheduled volume backup skipped; another backup is running", "volume", policy.VolumeName)
		return scheduler.Outcome{Status: scheduler.Skipped}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "Scheduled volume backup failed", "volume", policy.VolumeName, "error", err)
		return scheduler.Outcome{}, publicPolicyErrorInternal(err, 500)
	}
	slog.InfoContext(ctx, "Scheduled volume backup completed", "volume", policy.VolumeName, "backupId", entry.ID)
	return scheduler.Outcome{Status: scheduler.Succeeded, ActivityID: activityID}, nil
}

// publicPolicyErrorInternal keeps typed refusals; any other failure leaves the
// host as a stable code, so run history, activities and responses carry no diagnostics.
func publicPolicyErrorInternal(err error, status int) error {
	if err == nil {
		return nil
	}
	var typed huma.StatusError
	if errors.As(err, &typed) {
		return typed
	}
	return backupdto.NewProblem(status, "volume.backup.interrupted", err)
}

func (s *Service) rescheduleInternal(ctx context.Context, policy *VolumeBackupPolicy) {
	if policy == nil {
		return
	}
	if !policy.Enabled {
		s.jobs.Unregister(ctx, policy.ID)
		return
	}
	policyID := policy.ID
	s.jobs.Register(ctx, policyID,
		func(ctx context.Context) string {
			var current VolumeBackupPolicy
			if err := s.deps.DB.WithContext(ctx).Where("id = ?", policyID).First(&current).Error; err != nil {
				return defaultSchedule
			}
			return current.Schedule
		},
		func(ctx context.Context) (scheduler.Outcome, error) {
			return s.runScheduledBackupInternal(ctx, policyID)
		},
		func(ctx context.Context, previous scheduler.Run) (scheduler.Outcome, error) {
			return s.deps.Reconcile(ctx, previous, policy.VolumeName)
		},
	)
}

func (s *Service) RegisterJobsOnStartup(ctx context.Context) {
	if !s.jobs.Enabled() {
		return
	}
	var policies []VolumeBackupPolicy
	if err := s.deps.DB.WithContext(ctx).Where("enabled = ?", true).Find(&policies).Error; err != nil {
		slog.ErrorContext(ctx, "Failed to load scheduled volume backups", "error", err)
		return
	}
	for i := range policies {
		s.rescheduleInternal(ctx, &policies[i])
	}
	slog.InfoContext(ctx, "Registered scheduled volume backup jobs", "count", len(policies))
}

// Remove unregisters and deletes every policy of a removed volume.
func (s *Service) Remove(ctx context.Context, volumeName string) {
	policies, err := s.policiesInternal(ctx, volumeName)
	if err != nil {
		return
	}
	for i := range policies {
		s.jobs.Unregister(ctx, policies[i].ID)
	}
	if deleteBackupPolicyErr := s.deps.DB.WithContext(ctx).Where("volume_name = ?", volumeName).Delete(&VolumeBackupPolicy{}).Error; deleteBackupPolicyErr != nil {
		slog.WarnContext(ctx, "Failed to delete volume backup policy", "volume", volumeName, "error", deleteBackupPolicyErr)
	}
}

// Rename moves a renamed volume's policies inside the caller's transaction.
func (s *Service) Rename(tx *gorm.DB, oldName, newName string) error {
	return tx.Model(&VolumeBackupPolicy{}).Where("volume_name = ?", oldName).Update("volume_name", newName).Error
}

// AdmissionLock shares policy/reference admission with the owning-host backup service.
func (s *Service) AdmissionLock()   { s.admission.Lock() }
func (s *Service) AdmissionUnlock() { s.admission.Unlock() }

// NextRun returns scheduler knowledge from the process that owns this policy.
func (s *Service) NextRun(id string, enabled bool) *time.Time {
	if !enabled {
		return nil
	}
	if runtime, ok := s.jobs.Scheduler().(interface {
		GetJobRuntimeState(string) (scheduler.JobRuntimeState, bool)
	}); ok {
		if state, found := runtime.GetJobRuntimeState(s.jobs.JobName(id)); found {
			return state.NextRun
		}
	}
	return nil
}
