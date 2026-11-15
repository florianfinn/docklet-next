// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package hostio

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

type Epoch struct {
	ContainerID string
	StartedAt   string
	Running     bool
}
type Inspect func(context.Context) (Epoch, error)
type LogFence struct {
	Expected Epoch
	Inspect  Inspect
}

func (f LogFence) Check(ctx context.Context) error {
	got, err := f.Inspect(ctx)
	if err != nil {
		return err
	}
	if got.ContainerID != f.Expected.ContainerID || got.StartedAt != f.Expected.StartedAt {
		return dto.Fail(dto.ContainerChanged, 409)
	}
	return nil
}
func (f LogFence) Batch(ctx context.Context, read func(context.Context) ([]byte, error)) ([]byte, error) {
	if err := f.Check(ctx); err != nil {
		return nil, err
	}
	b, err := read(ctx)
	if err != nil {
		return nil, dto.Fail(dto.LogsUnavailable, 409)
	}
	if err = f.Check(ctx); err != nil {
		return nil, err
	}
	return b, nil
}
func (f LogFence) Settle(ctx context.Context) error { return f.Check(ctx) }

// Restart is used only after the operation has persisted restarting and the
// operator's acknowledgement. StartedAt is captured before Docker is called.
func Restart(ctx context.Context, allowed bool, inspect Inspect, restart func(context.Context) error) (string, error) {
	if !allowed {
		return "", dto.Fail(dto.AcknowledgementRequired, 409)
	}
	before, err := inspect(ctx)
	if err != nil {
		return "", err
	}
	if err = restart(ctx); err != nil {
		return before.StartedAt, err
	}
	after, err := inspect(ctx)
	if err != nil {
		return before.StartedAt, err
	}
	if after.ContainerID != before.ContainerID || !after.Running || after.StartedAt == before.StartedAt {
		return before.StartedAt, dto.Fail(dto.ContainerChanged, 409)
	}
	return before.StartedAt, nil
}
