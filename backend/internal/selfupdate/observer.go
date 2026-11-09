// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"os"
	"time"

	"gorm.io/gorm"
)

type Observer struct {
	Store   *Store
	Data    string
	Version func() string
	Running func(context.Context, Operation) (bool, error)
	Cleanup func(context.Context, Operation) error
	Settle  func(*gorm.DB, Operation) error
	Now     func() time.Time
}

// Advance does not infer success from a restarted binary alone. Terminal files
// belong to one immutable operation and are joined with the running version.
func (o Observer) Advance(ctx context.Context) (bool, error) {
	op, err := o.Store.Read(ctx)
	if err != nil || op == nil {
		return true, err
	}
	if op.Phase == "settled" {
		if o.Cleanup != nil {
			return true, o.Cleanup(ctx, *op)
		}
		return true, nil
	}
	var result Result
	fileErr := ReadJSON(ResultPath(o.Data, op.OpID), &result)
	matched := fileErr == nil && result.OpID == op.OpID && result.TargetRef == op.TargetRef
	outcome, code := "", ""
	if matched {
		switch result.Phase {
		case "healthy":
			if o.Version() == op.TargetVersion {
				outcome = "healthy"
			}
		case "failed-health", "failed-pre-effect":
			outcome = result.Phase
			code = CodeFailed
		}
	}
	if outcome == "" && matched && (result.Phase == "env-written" || result.Phase == "up-started") {
		running, e := o.Running(ctx, *op)
		if e == nil && !running {
			outcome = "unknown"
			code = CodeUnknown
		}
	}
	now := time.Now()
	if o.Now != nil {
		now = o.Now()
	}
	if outcome == "" && !now.Before(op.Deadline) {
		running, err := o.Running(ctx, *op)
		if err != nil && now.Before(op.Deadline.Add(OperationTimeout)) {
			return false, err
		}
		if !running || !now.Before(op.Deadline.Add(OperationTimeout)) {
			outcome = "unknown"
			code = CodeUnknown
		}
	}
	if outcome == "" {
		return false, nil
	}
	_, err = o.Store.Change(ctx, op.OpID, func(tx *gorm.DB, current *Operation) error {
		current.Phase = "settled"
		current.Outcome = outcome
		current.Code = code
		if o.Settle != nil {
			return o.Settle(tx, *current)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	settled, err := o.Store.Read(ctx)
	if err != nil {
		return false, err
	}
	if settled != nil && settled.OpID == op.OpID && settled.Phase == "settled" && o.Cleanup != nil {
		err = o.Cleanup(ctx, *settled)
	}
	return true, err
}

// Run is owned by application lifetime, independent of readiness and requests.
// Persistence and cleanup errors retry; stopping never replays the helper.
func (o Observer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		done, err := o.Advance(ctx)
		if done && err == nil {
			return
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func RemoveRequest(data, id string) error {
	err := os.Remove(RequestPath(data, id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
