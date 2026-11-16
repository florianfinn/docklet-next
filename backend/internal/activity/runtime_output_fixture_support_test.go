// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered legacy streams preserve output and encode visible loss as valid NDJSON.

package activity

import (
	"context"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// DelayRuntimeOutputAppendForTest models an already queued output write. Closing
// the coordinator drains it; otherwise release delivers it after the overwrite.
func DelayRuntimeOutputAppendForTest(t *testing.T, c *RuntimeRemoteCoordinator, run string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(c.lifetime)
	task := &runtimeRemoteTaskInternal{cancel: cancel, done: make(chan struct{})}
	release := make(chan struct{})
	result := make(chan error, 1)
	key := run + "/fixture-output"
	c.mu.Lock()
	c.follows[key] = task
	c.mu.Unlock()
	go func() {
		defer close(task.done)
		defer cancel()
		select {
		case <-ctx.Done():
		case <-release:
		}
		result <- c.service.activity.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
			var row Activity
			if err := tx.First(&row, "id = ?", run).Error; err != nil {
				return err
			}
			updates := map[string]any{}
			if err := runtimeAppendOutputTxInternal(tx, &row, updates, []activitytypes.OutputLine{{Timestamp: time.Now().UTC(), Stream: "stdout", Text: `{"log":"delayed progress"}`}}, true); err != nil {
				return err
			}
			return tx.Model(&Activity{}).Where("id = ?", run).UpdateColumns(updates).Error
		})
		c.mu.Lock()
		delete(c.follows, key)
		c.mu.Unlock()
	}()
	return func() {
		close(release)
		select {
		case <-task.done:
			require.NoError(t, <-result)
		case <-time.After(5 * time.Second):
			t.Fatal("delayed output append did not finish")
		}
	}
}
