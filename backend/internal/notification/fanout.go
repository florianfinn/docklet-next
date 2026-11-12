// Source: getarcaneapp/arcane/backend/internal/notification/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-11-06
// License: BSD-3-Clause
// Deviations: Extracted existing fan-out with M19 per-target delivery metadata.

package notification

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"strings"
)

// notifyEnabledProvidersInternal is the single fan-out loop behind every
// notification event: it walks all provider settings, skips disabled providers
// and providers with the event unsubscribed, dispatches to the rest, logs each
// attempt, and aggregates send errors. It returns how many providers the
// notification was actually delivered to.
func (s *NotificationService) notifyEnabledProvidersInternal(
	ctx context.Context,
	target NotificationTarget,
	eventType notifications.NotificationEventType,
	logRef string,
	metadata database.JSON,
	dispatch func(ctx context.Context, provider notifications.NotificationProvider, config database.JSON) (handled bool, err error),
) (int, error) {
	settings, err := s.GetAllSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get notification settings: %w", err)
	}

	eligible := make([]NotificationSettings, 0, len(settings))
	for _, setting := range settings {
		if setting.Enabled && s.isEventEnabled(setting.Config, eventType) {
			eligible = append(eligible, setting)
		}
	}

	handled := make([]bool, len(eligible))
	sendErrs := make([]error, len(eligible))
	var g errgroup.Group
	g.SetLimit(notificationDispatchConcurrencyInternal)
	for i, setting := range eligible {
		g.Go(func() error {
			defer utils.RecoverToError(&sendErrs[i], "notification dispatch", "provider", setting.Provider)
			handled[i] = true
			handled[i], sendErrs[i] = dispatch(ctx, setting.Provider, setting.Config)
			return nil
		})
	}
	var errs []string
	if waitErr := g.Wait(); waitErr != nil {
		errs = append(errs, fmt.Sprintf("notification dispatch: %v", waitErr))
	}

	delivered := 0
	if entry, ok := ctx.Value(imagePolicyEntryKey{}).(string); ok {
		metadata["entryId"] = entry
	}
	for i, setting := range eligible {
		if !handled[i] {
			slog.WarnContext(ctx, "Unknown notification provider", "provider", setting.Provider)
			continue
		}
		if sendErrs[i] == nil {
			delivered++
		}
		status, errMsg := collectNotificationSendResultInternal(&errs, setting.Provider, sendErrs[i])
		rowMetadata := cloneNotificationConfigInternal(metadata)
		rowMetadata["targetId"], rowMetadata["targetName"] = setting.ID, setting.Name
		s.logNotificationInternal(ctx, target.EnvironmentID, setting.Provider, logRef, status, errMsg, rowMetadata)
	}

	if len(errs) > 0 {
		return delivered, fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return delivered, nil
}
