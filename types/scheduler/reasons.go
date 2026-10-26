// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Reject undeclared durable reason codes at the persistence boundary.

package scheduler

import "github.com/florianfinn/docklet-next/types/image"

func (o Outcome) ValidateReasons() error {
	if err := image.ValidateReason(o.Code); err != nil {
		return err
	}
	for _, target := range o.Targets {
		if err := image.ValidateReason(target.Code); err != nil {
			return err
		}
	}
	return nil
}

func (r Run) ValidateReasons() error {
	if err := r.Outcome.ValidateReasons(); err != nil {
		return err
	}
	for _, attempt := range r.Attempts {
		if err := attempt.Outcome.ValidateReasons(); err != nil {
			return err
		}
	}
	if r.RemoteOutcome != nil {
		return r.RemoteOutcome.ValidateReasons()
	}
	return nil
}

func (r QueueRecord) ValidateReasons() error {
	for _, run := range r.Runs {
		if err := run.ValidateReasons(); err != nil {
			return err
		}
	}
	for _, run := range r.Receipts {
		if err := run.ValidateReasons(); err != nil {
			return err
		}
	}
	return nil
}
