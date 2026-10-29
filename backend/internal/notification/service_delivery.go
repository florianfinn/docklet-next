// Source: getarcaneapp/arcane/backend/internal/notification/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; split existing delivery and operator probes from service.

package notification

import (
	"cmp"
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/florianfinn/docklet-next/types/vulnerability"
	"go.getarcane.app/kit/pkg"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"
)

// SendBatchImageUpdateNotification dispatches a batched image-update notification
// and returns the number of eligible providers it was delivered to (0 means no
// provider has this event enabled, so callers must not mark the updates notified).
func (s *NotificationService) SendBatchImageUpdateNotification(ctx context.Context, updates map[string]*imageupdate.Response) (int, error) {
	updatesWithChanges := filterUpdatesWithChangesInternal(updates)
	if len(updatesWithChanges) == 0 {
		return 0, nil
	}

	if s.config != nil && s.config.AgentMode {
		dispatchResponse, err := s.dispatchNotificationToManagerInternal(ctx, notification.DispatchRequest{
			Kind: notification.DispatchKindBatchImageUpdate,
			BatchImageUpdate: &notification.DispatchBatchImageUpdate{
				Updates: updatesWithChanges,
			},
		})
		if err != nil {
			return 0, err
		}
		return dispatchResponse.Delivered, nil
	}

	target, err := s.resolveNotificationTargetInternal(ctx, "")
	if err != nil {
		return 0, err
	}

	return s.sendBatchImageUpdateNotificationForTargetInternal(ctx, target, updatesWithChanges)
}

func filterUpdatesWithChangesInternal(updates map[string]*imageupdate.Response) map[string]*imageupdate.Response {
	updatesWithChanges := make(map[string]*imageupdate.Response, len(updates))
	for imageRef, update := range updates {
		if update != nil && update.HasUpdate {
			updatesWithChanges[imageRef] = update
		}
	}
	return updatesWithChanges
}

func (s *NotificationService) sendBatchImageUpdateNotificationForTargetInternal(ctx context.Context, target NotificationTarget, updates map[string]*imageupdate.Response) (int, error) {
	updatesWithChanges := filterUpdatesWithChangesInternal(updates)

	if len(updatesWithChanges) == 0 {
		return 0, nil
	}

	imageRefs := slices.Collect(maps.Keys(updatesWithChanges))

	metadata := database.JSON{
		"updateCount": len(updatesWithChanges),
		"eventType":   string(notifications.NotificationEventImageUpdate),
		"batch":       true,
	}
	content := s.templates.BatchImageUpdate(target.EnvironmentName, updatesWithChanges)
	content.Vars = notifications.EventVars(target.EnvironmentName, target.EnvironmentID, notifications.NotificationEventImageUpdate)
	return s.notifyEnabledProvidersInternal(ctx, target, notifications.NotificationEventImageUpdate, strings.Join(
		imageRefs,
		", ",
	), metadata, func(
		ctx context.Context,
		provider notifications.NotificationProvider,
		config database.JSON,
	) (
		bool,
		error,
	) {
		return notifications.Deliver(ctx, provider, config, content)
	})
}

func (s *NotificationService) SendPruneReportNotification(ctx context.Context, result *system.PruneAllResult) error {
	if result == nil {
		slog.InfoContext(ctx, "skipping prune report notification because no prune result was reported")
		return nil
	}

	hasChanges := pruneResultHasChangesInternal(result)
	hasErrors := len(result.Errors) > 0
	if !hasChanges && !hasErrors {
		slog.InfoContext(ctx, "skipping prune report notification because no resources were pruned and no errors were reported")
		return nil
	}

	if s.config != nil && s.config.AgentMode {
		_, err := s.dispatchNotificationToManagerInternal(ctx, notification.DispatchRequest{
			Kind: notification.DispatchKindPruneReport,
			PruneReport: &notification.DispatchPruneReport{
				Result: *result,
			},
		})
		return err
	}

	target, err := s.resolveNotificationTargetInternal(ctx, "")
	if err != nil {
		return err
	}

	return s.sendPruneReportNotificationForTargetInternal(ctx, target, result)
}

func (s *NotificationService) sendPruneReportNotificationForTargetInternal(ctx context.Context, target NotificationTarget, result *system.PruneAllResult) error {
	hasChanges := pruneResultHasChangesInternal(result)
	hasErrors := len(result.Errors) > 0

	metadata := database.JSON{
		"spaceReclaimed": result.SpaceReclaimed,
		"eventType":      string(notifications.NotificationEventPruneReport),
	}
	content := s.templates.PruneReport(target.EnvironmentName, result)
	content.Vars = notifications.EventVars(target.EnvironmentName, target.EnvironmentID, notifications.NotificationEventPruneReport)
	_, err := s.notifyEnabledProvidersInternal(ctx, target, notifications.NotificationEventPruneReport, "System Prune Report", metadata, func(
		ctx context.Context,
		provider notifications.NotificationProvider,
		config database.JSON,
	) (
		bool,
		error,
	) {
		return notifications.Deliver(ctx, provider, config, content)
	})
	if err != nil {
		return err
	}
	if hasErrors && !hasChanges {
		slog.WarnContext(ctx, "sending prune report notification with errors but no resources were pruned", "errorCount", len(result.Errors))
	}

	return nil
}

func pruneResultHasChangesInternal(result *system.PruneAllResult) bool {
	if result == nil {
		return false
	}

	if result.SpaceReclaimed > 0 {
		return true
	}

	return len(result.ContainersPruned) > 0 ||
		len(result.ImagesDeleted) > 0 ||
		len(result.VolumesDeleted) > 0 ||
		len(result.NetworksDeleted) > 0
}

// SendAutoHealNotification sends a notification when a container is auto-healed.
func (s *NotificationService) SendAutoHealNotification(ctx context.Context, containerName, containerID string) error {
	if s.config != nil && s.config.AgentMode {
		_, err := s.dispatchNotificationToManagerInternal(ctx, notification.DispatchRequest{
			Kind: notification.DispatchKindAutoHeal,
			AutoHeal: &notification.DispatchAutoHeal{
				ContainerName: containerName,
				ContainerID:   containerID,
			},
		})
		return err
	}

	target, err := s.resolveNotificationTargetInternal(ctx, "")
	if err != nil {
		return err
	}

	return s.sendAutoHealNotificationForTargetInternal(ctx, target, containerName, containerID)
}

func (s *NotificationService) sendAutoHealNotificationForTargetInternal(ctx context.Context, target NotificationTarget, containerName, containerID string) error {
	metadata := database.JSON{
		"containerID": containerID,
		"eventType":   string(notifications.NotificationEventAutoHeal),
	}
	content := s.templates.AutoHeal(target.EnvironmentName, containerName)
	content.Vars = notifications.EventVars(target.EnvironmentName, target.EnvironmentID, notifications.NotificationEventAutoHeal)
	_, err := s.notifyEnabledProvidersInternal(ctx, target, notifications.NotificationEventAutoHeal, containerName, metadata, func(
		ctx context.Context,
		provider notifications.NotificationProvider,
		config database.JSON,
	) (
		bool,
		error,
	) {
		return notifications.Deliver(ctx, provider, config, content)
	})
	return err
}

// --- Test notifications ---

// notificationEventTypeForTestTypeInternal maps a test type to the event type a
// real notification of that kind would be gated on ("" = no event gate).
func notificationEventTypeForTestTypeInternal(testType string) notifications.NotificationEventType {
	switch testType {
	case notificationTestTypeImageUpdate, notificationTestTypeBatchImageUpdate:
		return notifications.NotificationEventImageUpdate
	case notificationTestTypeVulnerability:
		return notifications.NotificationEventVulnerabilityFound
	case notificationTestTypePruneReport:
		return notifications.NotificationEventPruneReport
	case notificationTestTypeAutoHeal:
		return notifications.NotificationEventAutoHeal
	default:
		return ""
	}
}

// testNotificationWarningInternal reports why a real notification would not send
// even though the test did: the provider is disabled, or the tested event type is
// unsubscribed. Empty means real notifications would send.
func (s *NotificationService) testNotificationWarningInternal(setting *NotificationSettings, testType string) string {
	if !setting.Enabled {
		return "notification.target-disabled"
	}
	eventType := notificationEventTypeForTestTypeInternal(testType)
	if eventType != "" && !s.isEventEnabled(setting.Config, eventType) {
		return "notification.target-disabled"
	}
	if eventType == "" && !slices.ContainsFunc(notifications.AllNotificationEventTypes, func(candidate notifications.NotificationEventType) bool {
		return s.isEventEnabled(setting.Config, candidate)
	}) {
		return "notification.target-disabled"
	}
	return ""
}

func (s *NotificationService) testNotificationContentInternal(environmentName, testType string) notifications.Content {
	switch testType {
	case notificationTestTypeVulnerability:
		return s.templates.VulnerabilityScan(notification.DispatchVulnerabilityScan{HostName: environmentName, Outcome: "partial", NewCritical: 1, NewHigh: 3, NewFindings: []vulnerability.NewFinding{{VulnerabilityID: "CVE-2026-1000", ImageName: "example.com/image:1"}}})
	case notificationTestTypeAutoHeal:
		return s.templates.AutoHeal(environmentName, "test-container")
	case notificationTestTypePruneReport:
		return s.templates.PruneReport(environmentName, &system.PruneAllResult{
			Success:                  true,
			ContainersPruned:         []string{"a1b2c3d4e5f6", "f6e5d4c3b2a1"},
			ImagesDeleted:            []string{"sha256:1111111111111111111111111111111111111111111111111111111111111111"},
			VolumesDeleted:           []string{"arcane_test_volume"},
			NetworksDeleted:          []string{"arcane_test_network"},
			SpaceReclaimed:           3825205248,
			ContainerSpaceReclaimed:  503316480,
			ImageSpaceReclaimed:      2449473536,
			VolumeSpaceReclaimed:     641728512,
			BuildCacheSpaceReclaimed: 230162432,
			Errors:                   []string{},
		})
	case notificationTestTypeBatchImageUpdate:
		return s.templates.BatchImageUpdate(environmentName, map[string]*imageupdate.Response{
			"nginx:latest": {
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentDigest:  "sha256:abc123def456789012345678901234567890",
				LatestDigest:   "sha256:xyz789ghi012345678901234567890123456",
				CheckTime:      time.Now(),
				ResponseTimeMs: 100,
			},
			"postgres:16-alpine": {
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentDigest:  "sha256:def456abc123789012345678901234567890",
				LatestDigest:   "sha256:ghi789xyz012345678901234567890123456",
				CheckTime:      time.Now(),
				ResponseTimeMs: 120,
			},
			"redis:7.2-alpine": {
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentDigest:  "sha256:123456789abc012345678901234567890def",
				LatestDigest:   "sha256:456789012def345678901234567890123abc",
				CheckTime:      time.Now(),
				ResponseTimeMs: 95,
			},
		})
	default: // simple and image-update
		imageRef := kit.Ternary(testType == notificationTestTypeSimple, "test/image:latest", "nginx:latest")
		return s.templates.ImageUpdate(environmentName, imageRef, &imageupdate.Response{
			HasUpdate:      true,
			UpdateType:     "digest",
			CurrentDigest:  "sha256:abc123def456789012345678901234567890",
			LatestDigest:   "sha256:xyz789ghi012345678901234567890123456",
			CheckTime:      time.Now(),
			ResponseTimeMs: 100,
		})
	}
}

// TestNotification sends a test message to the provider regardless of its enabled
// state (testing before enabling is legitimate) and returns a warning when a real
// notification of the tested kind would not send.
func (s *NotificationService) TestNotification(ctx context.Context, environmentID string, provider notifications.NotificationProvider, testType string) (string, error) {
	setting, err := s.GetSettingsByProvider(ctx, provider)
	if err != nil {
		return "", fmt.Errorf("please save your %s settings before testing", provider)
	}
	testType = cmp.Or(strings.TrimSpace(testType), notificationTestTypeSimple)
	if _, ok := supportedNotificationTestTypes[testType]; !ok {
		return "", fmt.Errorf("unsupported notification test type: %s", testType)
	}
	warning := s.testNotificationWarningInternal(setting, testType)

	target, err := s.resolveNotificationTargetInternal(ctx, environmentID)
	if err != nil {
		return "", err
	}

	if provider == notifications.NotificationProviderEmail && testType == notificationTestTypeSimple {
		_, sendErr := notifications.Deliver(ctx, notifications.NotificationProviderEmail, setting.Config, s.templates.TestEmail(target.EnvironmentName))
		return warning, sendErr
	}

	content := s.testNotificationContentInternal(target.EnvironmentName, testType)
	// Stamp the event vars so the Test button exercises a configured generic
	// payload template. The simple test renders image-update content, so it
	// reports that event type.
	testEventType := cmp.Or(notificationEventTypeForTestTypeInternal(testType), notifications.NotificationEventImageUpdate)
	content.Vars = notifications.EventVars(target.EnvironmentName, target.EnvironmentID, testEventType)
	handled, sendErr := notifications.Deliver(ctx, provider, setting.Config, content)
	if !handled {
		return "", fmt.Errorf("unknown provider: %s", provider)
	}
	return warning, sendErr
}
