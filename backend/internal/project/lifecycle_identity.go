// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: All-replica no-op and frozen identity guards.

package project

import (
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func lifecycleSelectionCurrent(snap *life.Snapshot, sel dto.ImageSelection) bool {
	expected, err := identity.ParseExpected(sel.ImageID)
	if err != nil {
		return false
	}
	count := 0
	for _, img := range snap.Containers {
		if img.Service != sel.Service || img.OneOff {
			continue
		}
		count++
		local, err := identity.ParseLocalID(img.ImageID)
		if err != nil || !identity.MatchesExpected(local, expected) {
			return false
		}
	}
	service, ok := snap.Model.Services[sel.Service]
	minimum := 1
	if ok && service.Deploy != nil && service.Deploy.Replicas != nil {
		minimum = int(*service.Deploy.Replicas)
	}
	return count > 0 && count >= minimum
}
func lifecycleValidateSelections(selections []dto.ImageSelection) error {
	for _, sel := range selections {
		if _, err := identity.ParseExpected(sel.ImageID); err != nil {
			return lifecycleFailure("update.target-changed")
		}
		if _, err := identity.ParsePlatform(sel.Platform); err != nil {
			return lifecycleFailure("update.target-changed")
		}

	}
	return nil
}

func lifecycleExecutableSelections(snap *life.Snapshot, selections []dto.ImageSelection, restore bool) ([]dto.ImageSelection, error) {
	if err := lifecycleValidateSelections(selections); err != nil {
		return nil, err
	}
	if restore {
		return selections, nil
	}
	out := []dto.ImageSelection{}
	for _, sel := range selections {
		if !lifecycleSelectionCurrent(snap, sel) {
			out = append(out, sel)
		}
	}
	if len(selections) > 0 && len(out) == 0 {
		return nil, lifecycleFailure("job.skipped")
	}
	return out, nil
}

func lifecycleMatchesExpected(actual, expected life.ServiceImage) bool {
	local, e := identity.ParseLocalID(actual.ImageID)
	frozen, f := identity.ParseExpected(expected.ImageID)
	p, pe := identity.ParsePlatform(actual.Platform)
	want, we := identity.ParsePlatform(expected.Platform)
	return e == nil && f == nil && pe == nil && we == nil && identity.MatchesExpected(local, frozen) && p.Matches(want) && actual.Digest == expected.Digest
}
