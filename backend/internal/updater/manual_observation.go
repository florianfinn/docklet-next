// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub notification ownership for manual image checks.

package updater

import (
	"context"
	"github.com/distribution/reference"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
)

func sameImageReference(a, b string) bool {
	x, e := reference.ParseNormalizedNamed(a)
	if e != nil {
		return false
	}
	y, e := reference.ParseNormalizedNamed(b)
	return e == nil && reference.TagNameOnly(x).String() == reference.TagNameOnly(y).String()
}

func (s *UpdaterService) observeManualImages(ctx context.Context, env string) (func(checkDTO.BatchResponse) error, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return nil, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return nil, err
	}
	targets, err := p.ports.targets(ctx, env)
	if err != nil {
		return nil, err
	}
	rows := map[string]policy.Policy{}
	for _, target := range targets {
		if target.Kind != "container" || !target.Eligible {
			continue
		}
		row, e := p.store.Policy(ctx, env, reg, target.ID, "container")
		if e != nil {
			return nil, e
		}
		if row.Revision == 0 {
			row.TargetIdentity = target.Name
			row, e = p.store.EnsureObservedPolicy(ctx, row)
			if e != nil {
				return nil, e
			}
		}
		rows[target.ID] = row
	}
	return func(batch checkDTO.BatchResponse) error {
		if len(batch) == 0 {
			return nil
		}
		// Image responses observe standalone containers only: a single image result cannot
		// replace a stack's candidate set, and stacks are observed by their own checks.
		// One failing target never stops the others; the first error is reported at the end.
		var first error
		for _, target := range targets {
			if !target.Eligible || target.Kind != "container" {
				continue
			}
			for ref, result := range batch {
				if result == nil || !sameImageReference(ref, target.ImageRef) && !sameImageReference(result.ImageRef, target.ImageRef) {
					continue
				}
				check, e := containerCheckProjection(target, result)
				if e == nil && check.Status != "unknown" {
					row := rows[target.ID]
					if _, e = s.observeImagePolicy(ctx, p, row, check); e == nil {
						s.recordImagePolicyStatus(ctx, p, row, "update-check", policyTarget(target.ID, "succeeded", ""))
					}
				}
				if e != nil && first == nil {
					first = e
				}
				break
			}
		}
		return first
	}, nil
}
