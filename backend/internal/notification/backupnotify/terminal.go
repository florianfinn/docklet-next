// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backupnotify

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"log/slog"
)

type Dispatch func(context.Context, dto.DispatchRequest) error
type Notifier struct {
	DB       *database.DB
	Agent    bool
	Dispatch Dispatch
}
type Run struct {
	ID, EnvironmentID, Host, Operation, Volume, Reason, ActivityID, Status string
	Suppress                                                               bool
}

// Terminal runs after a live terminal save. History repair never calls it.
func (n Notifier) Terminal(ctx context.Context, r Run) {
	if r.Suppress || ctx.Err() != nil || (r.Status != "failed" && r.Status != "partial" && r.Status != "interrupted") {
		return
	}
	if r.EnvironmentID == "" {
		r.EnvironmentID = "0"
	}
	p := dto.DispatchBackupFailed{EventID: fmt.Sprintf("%x", sha256.Sum256([]byte(r.Operation+":"+r.ID))), Operation: r.Operation, Volume: r.Volume, Reason: r.Reason, ActivityID: r.ActivityID}
	if p.Reason == "" {
		p.Reason = "backup.failed"
	}
	var err error
	if n.Agent {
		if n.Dispatch == nil {
			err = errors.New("notification.dispatch-unavailable")
		} else {
			err = n.Dispatch(ctx, dto.DispatchRequest{Kind: dto.DispatchKindBackupFailed, BackupFailed: &p})
		}
	} else if n.DB != nil {
		err = notificationoutbox.Insert(n.DB.WithContext(context.WithoutCancel(ctx)), notificationoutbox.Row{Event: "backup_failed", EnvironmentID: r.EnvironmentID, DedupKey: r.Operation + ":" + r.EnvironmentID + ":" + r.ID, Payload: database.JSON{"host": r.Host, "operation": r.Operation, "volume": r.Volume, "reason": p.Reason, "activityId": r.ActivityID}})
	}
	if err != nil {
		slog.WarnContext(ctx, "Backup failure notification could not be accepted", "code", "notification.dispatch-failed")
	}
}

func ActivityID(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}
