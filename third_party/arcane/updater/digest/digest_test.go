// Source: getarcaneapp/kit/updater/digest/digest_test.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package digest

import (
	"testing"

	ocidigest "github.com/opencontainers/go-digest"
)

func TestFromReferenceSuffix(t *testing.T) {
	want := ocidigest.FromString("arcane-reference").String()

	got, ok := FromReferenceSuffix("docker.io/library/nginx@" + want)
	if !ok {
		t.Fatal("FromReferenceSuffix() ok = false, want true")
	}
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}

	if _, ok = FromReferenceSuffix("docker.io/library/nginx@sha256:bad"); ok {
		t.Fatal("FromReferenceSuffix() ok = true for invalid digest")
	}
}
