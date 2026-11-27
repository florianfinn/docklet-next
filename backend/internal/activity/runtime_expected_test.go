// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package activity

import (
	"fmt"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestExpectedLimitsAndOwnership(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeUpdateProject)
	binding := &dto.PolicyBinding{PolicyID: "policy", Revision: 1, JobRunID: "window", ChildID: uuid.NewString()}
	c.Options.Lifecycle.Policy = binding
	for i := 0; i < dto.MaxLifecycleServices; i++ {
		binding.Expected = append(binding.Expected, dto.ExpectedCandidate{Service: fmt.Sprint("service", i), Manifest: "sha256:" + strings.Repeat("a", 64)})
	}
	canonical, e := runtimeCanonicalizeInternal(c)
	require.NoError(t, e)
	binding.Expected[0].Manifest = "sha256:" + strings.Repeat("b", 64)
	require.NotEqual(t, binding.Expected[0].Manifest, canonical.command.Options.Lifecycle.Policy.Expected[0].Manifest)
	binding.Expected = append(binding.Expected, dto.ExpectedCandidate{Service: "extra", Manifest: "sha256:" + strings.Repeat("a", 64)})
	_, e = runtimeCanonicalizeInternal(c)
	require.Error(t, e)
	binding.Expected = binding.Expected[:1]
	binding.Expected[0].Manifest = "invalid"
	_, e = runtimeCanonicalizeInternal(c)
	require.Error(t, e)
}
