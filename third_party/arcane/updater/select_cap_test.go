// Source: docklet-next/issues/677
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original bound for platform candidate lookups.

package updater

import (
	"context"
	"fmt"
	"testing"

	"go.getarcane.app/updater/types"
)

func TestSelectTagCandidateBoundsRefusedLookups(t *testing.T) {
	tags := []string{"1.0.0"}
	for i := 1; i <= 12; i++ {
		tags = append(tags, fmt.Sprintf("1.%d.0", i))
	}
	lookups := 0
	selected, err := SelectTagCandidate(t.Context(), "1.0.0", tags, types.Policy{Strategy: "tag"}, func(context.Context, string) (bool, error) {
		lookups++
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if lookups != maxRefusedTagCandidates {
		t.Fatalf("lookups = %d, want %d", lookups, maxRefusedTagCandidates)
	}
	if selected != "1.4.0" {
		t.Fatalf("selected = %q, want the newest candidate after the bound", selected)
	}
}
