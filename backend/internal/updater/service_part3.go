// Source: getarcaneapp/arcane/backend/internal/updater/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package updater

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	"strings"

	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/containerd/errdefs"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/scheduler"
	arcaneupdater "github.com/florianfinn/docklet-next/types/updater"

	"github.com/moby/moby/client"

	"go.getarcane.app/updater"

	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"go.getarcane.app/updater/refs"
	updatertypes "go.getarcane.app/updater/types"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	projectpkg "github.com/florianfinn/docklet-next/backend/internal/project"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

// UpdateServiceImages refuses the retired engine-owned Compose update path.
func (s *UpdaterService) UpdateServiceImages(ctx context.Context, projectID string, changes map[string]updatertypes.ServiceImageChange) error {
	return imageDTO.Failure("update.path-retired", 409)
}

func (s *UpdaterService) scopedPendingRecordsInternal(ctx context.Context, records []imageupdate.ImageUpdateRecord) ([]updater.ImageUpdateRecord, error) {
	var scopedCount int64
	if err := s.deps.DB.WithContext(ctx).Model(&imageupdate.ImageUpdateRecord{}).Where("container_id <> ?", "").Limit(1).Count(&scopedCount).Error; err != nil {
		return nil, err
	}
	hasScoped := scopedCount > 0
	if !hasScoped {
		out := make([]updater.ImageUpdateRecord, 0, len(records))
		for _, record := range records {
			out = append(out, imageUpdateRecordToModuleInternal(record))
		}
		return out, nil
	}
	dockerClient, err := s.DockerClient(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{All: false})
	if err != nil {
		return nil, err
	}
	out := make([]updater.ImageUpdateRecord, 0, len(records))
	for _, record := range records {
		converted := imageUpdateRecordToModuleInternal(record)
		if record.ContainerID != "" {
			out = append(out, converted)
			continue
		}
		for _, cnt := range listed.Items {
			resolved, policyErr := tagpolicy.Resolve(cnt.Image, updater.DefaultLabelPolicy().TagPolicy(cnt.Labels))
			if policyErr != nil || resolved.Strategy == "tag" {
				continue
			}
			if refs.NormalizeImageUpdateRef(cnt.Image) != refs.NormalizeImageUpdateRef(converted.ImageRef()) {
				continue
			}
			converted.ContainerID = cnt.ID
			out = append(out, converted)
		}
	}
	return out, nil
}

func (s *UpdaterService) clearUnscopedRecordInternal(ctx context.Context, record updater.ImageUpdateRecord) error {
	dockerClient, err := s.DockerClient(ctx)
	if err != nil {
		return err
	}
	target, err := dockerClient.ImageInspect(ctx, record.NewImageRef())
	if err != nil {
		return err
	}
	listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{All: false})
	if err != nil {
		return err
	}
	for _, cnt := range listed.Items {
		resolved, policyErr := tagpolicy.Resolve(cnt.Image, updater.DefaultLabelPolicy().TagPolicy(cnt.Labels))
		if policyErr != nil || resolved.Strategy == "tag" {
			continue
		}
		if refs.NormalizeImageUpdateRef(cnt.Image) == refs.NormalizeImageUpdateRef(record.ImageRef()) && cnt.ImageID != target.ID {
			return nil
		}
	}
	return s.deps.DB.WithContext(ctx).Model(&imageupdate.ImageUpdateRecord{}).Where("id = ? AND container_id = ?", record.ID, "").Update("has_update", false).Error
}

// CheckProjectUpdates checks effective Compose service policies without deploying containers.
func (s *UpdaterService) CheckProjectUpdates(ctx context.Context, projectID string) (*projecttypes.UpdateInfo, error) {
	if s.deps.Projects == nil || s.deps.DB == nil || s.engine == nil {
		return nil, errors.New("project update checker unavailable")
	}
	details, err := s.deps.Projects.GetProjectDetails(ctx, projectID, projecttypes.DetailsOptions{IncludeServiceConfigs: true})
	if err != nil {
		return nil, err
	}
	if len(details.Services) == 0 {
		return nil, errors.New("project has no resolved Compose services to check")
	}
	records := make([]imageupdate.ImageUpdateRecord, 0, len(details.Services))
	for _, service := range details.Services {
		if strings.TrimSpace(service.Image) == "" {
			continue
		}
		records = append(records, s.checkProjectServiceInternal(ctx, projectID, service))
	}
	if transactionErr := s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if deleteProjectUpdatesErr := tx.Where("project_id = ?", projectID).Delete(&imageupdate.ImageUpdateRecord{}).Error; deleteProjectUpdatesErr != nil {
			return deleteProjectUpdatesErr
		}
		if len(records) == 0 {
			return nil
		}
		return tx.Create(&records).Error
	}); transactionErr != nil {
		return nil, fmt.Errorf("save project update checks: %w", transactionErr)
	}
	return projectpkg.BuildConfiguredUpdateInfo(projectID, details.Services, nil, records), nil
}

func (s *UpdaterService) checkProjectServiceInternal(ctx context.Context, projectID string, service types.ServiceConfig) imageupdate.ImageUpdateRecord {
	record := imageupdate.ImageUpdateRecord{
		ID:          "project::" + projectID + "::" + service.Name,
		ProjectID:   projectID,
		ServiceName: service.Name,
		PolicyKey: imageref.UpdatePolicyKey(
			service.Image,
			service.Labels,
		),
		CheckTime: time.Now().UTC(),
	}
	parsed, err := refs.NormalizeReference(service.Image)
	if err != nil {
		record.LastError = new(err.Error())
		return record
	}
	record.Repository = parsed.RegistryHost + "/" + parsed.Repository
	record.Tag = parsed.Tag
	record.CurrentVersion = parsed.Tag
	if service.Build != nil {
		record.UpdateType = imageupdate.UpdateTypeLocal
		return record
	}
	// Disabling automatic installation keeps the preview check; only the
	// update-check label opts a service out of monitoring.
	if imageref.IsUpdateCheckDisabled(service.Labels) {
		return record
	}
	check, err := s.engine.CheckImageUpdate(ctx, updatertypes.CheckRequest{ImageRef: service.Image, Policy: updater.DefaultLabelPolicy().TagPolicy(service.Labels)})
	if err != nil {
		if errdefs.IsNotFound(err) {
			record.UpdateType = imageupdate.UpdateTypeNotPulled
		} else {
			record.LastError = new(err.Error())
		}
		return record
	}
	record.HasUpdate = check.UpdateAvailable
	record.UpdateType = check.UpdateType
	record.CurrentVersion = cmp.Or(check.CurrentVersion, record.CurrentVersion)
	if check.CurrentDigest != "" {
		record.CurrentDigest = new(check.CurrentDigest)
	}
	if check.TargetDigest != "" {
		record.LatestDigest = new(check.TargetDigest)
	}
	if check.TargetRef != "" {
		target, parseErr := refs.NormalizeReference(check.TargetRef)
		if parseErr != nil {
			record.HasUpdate = false
			record.LastError = new(parseErr.Error())
		} else {
			record.LatestVersion = new(target.Tag)
		}
	}
	return record
}

func updateBatchTypeInternal(ctx context.Context, options arcaneupdater.Options, batchCompleted bool) string {
	batchType := "update-batch"
	if previous, ok := jobcontext.Run(ctx); ok && len(options.ResourceIds) > 0 {
		batchType = "update-retry"
		for _, target := range previous.Outcome.Targets {
			if target.ID == "auto-update" && target.ResourceType == "update-batch" && (target.Status == scheduler.Succeeded || target.Status == scheduler.Partial) {
				batchType = "update-batch"
			}
		}
	}
	if !batchCompleted {
		batchType = "update-interrupted"
	}
	return batchType
}

// RegisterActors registers durable single-container update delivery.
func (s *UpdaterService) RegisterActors(runtime *francis.Runtime) error {
	return s.execution.RegisterActors(runtime)
}

// Start repairs persisted single-container delivery intents until shutdown.
func (s *UpdaterService) Start(ctx context.Context) error {
	return s.execution.Start(ctx)
}

// Stop cancels business execution before the actor host drains jobs.
func (s *UpdaterService) Stop(ctx context.Context) error {
	return s.execution.Stop(ctx)
}

// ActiveUpdateActivityIDs lists activities owned by persisted single-container updates.
func (s *UpdaterService) ActiveUpdateActivityIDs(ctx context.Context) ([]string, error) {
	return s.execution.ActiveUpdateActivityIDs(ctx)
}

// ReconcilePending resumes only frozen targets whose original container is unchanged.
func (s *UpdaterService) ReconcilePending(ctx context.Context, run scheduler.Run) (scheduler.Outcome, error) {
	return s.recovery.ReconcilePending(ctx, run)
}
