// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original manual image observation isolation proof.

package updater

import (
	"context"
	"errors"
	"strings"
	"testing"

	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

// A failing stack on the same host never stops a manual image response from recording
// a standalone container candidate, and the response never fans out into stack checks.
func TestManualImageObservationRecordsContainerDespiteFailingStack(t *testing.T) {
	f := newPolicyFixture(t)
	ref := "registry.example.com/app:1.0.0"
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{
			{Kind: "stack", ID: "broken", Name: "broken", Eligible: true},
			{Kind: "container", ID: "app", Name: "app", ContainerID: "app-instance", ImageRef: ref, Eligible: true},
		}, nil
	}
	f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
		f.checks++
		return nil, errors.New("synthetic private image")
	}
	observe, e := f.s.observeManualImages(t.Context(), "0")
	require.NoError(t, e)
	batch := checkDTO.BatchResponse{ref: {ImageRef: ref, State: "version", HasUpdate: true, LatestVersion: "v1.1.0", LatestDigest: "sha256:" + strings.Repeat("c", 64), Platform: "linux/amd64"}}
	require.NoError(t, observe(batch))
	require.Zero(t, f.checks, "an image response must not re-check stacks")
	var row policy.Policy
	require.NoError(t, f.db.First(&row, "target_kind=? AND target_id=?", "container", "app").Error)
	require.Contains(t, row.Candidates, `"version":"1.1.0"`)
	require.Equal(t, 1, f.notifications)
}
