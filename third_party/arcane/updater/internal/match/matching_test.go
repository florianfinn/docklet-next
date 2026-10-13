// Source: getarcaneapp/kit/updater/internal/match/matching_test.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package match

import (
	"testing"

	"github.com/moby/moby/api/types/container"

	"go.getarcane.app/updater/refs"
)

func TestResolveContainerImageMatchUsesInspectConfigImage(t *testing.T) {
	updatedNorm := map[string]string{
		refs.NormalizeImageUpdateRef("nginx:1.27"): "nginx:1.28",
	}
	cnt := container.Summary{
		ID:      "container-1",
		Image:   "sha256:old",
		ImageID: "sha256:old",
	}
	inspect := &container.InspectResponse{
		Config: &container.Config{Image: "nginx:1.27"},
	}

	gotRef, gotMatch := ResolveContainerImageMatch(cnt, inspect, nil, updatedNorm)
	if gotRef != "nginx:1.28" {
		t.Fatalf("newRef = %q, want nginx:1.28", gotRef)
	}
	if gotMatch != refs.NormalizeImageUpdateRef("nginx:1.27") {
		t.Fatalf("match = %q, want normalized nginx:1.27", gotMatch)
	}
}

func TestAppendImageUpdateRecordIDToOldIDs(t *testing.T) {
	got := AppendImageUpdateRecordIDToOldIDs([]string{"sha256:one"}, "sha256:two")
	if len(got) != 2 || got[1] != "sha256:two" {
		t.Fatalf("AppendImageUpdateRecordIDToOldIDs() = %#v", got)
	}

	got = AppendImageUpdateRecordIDToOldIDs(got, "not-a-sha")
	if len(got) != 2 {
		t.Fatalf("non sha-like record ID was appended: %#v", got)
	}
}
