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
		for _, target := range targets {
			if !target.Eligible {
				continue
			}
			if target.Kind == "stack" {
				// A single image response cannot replace a stack's complete candidate set.
				// The ordinary stack check owns its own pre-read revision fence.
				if _, e := s.CheckPolicyProject(ctx, env, target.ID); e != nil {
					return e
				}
				continue
			}
			for ref, result := range batch {
				if result == nil || !sameImageReference(ref, target.ImageRef) && !sameImageReference(result.ImageRef, target.ImageRef) {
					continue
				}
				check, e := containerCheckProjection(target, result)
				if e != nil {
					return e
				}
				if check.Status == "unknown" {
					continue
				}
				row := rows[target.ID]
				if _, e = s.observeImagePolicy(ctx, p, row, check); e != nil {
					return e
				}
				s.recordImagePolicyStatus(ctx, p, row, "update-check", policyTarget(target.ID, "succeeded", ""))
				break
			}
		}
		return nil
	}, nil
}
