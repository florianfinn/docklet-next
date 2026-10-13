// Source: getarcaneapp/kit/pkg/ternary_test.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kit

import "testing"

func TestTernary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		condition bool
		want      int
	}{
		{name: "true chooses first value", condition: true, want: 10},
		{name: "false chooses second value", condition: false, want: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Ternary(tt.condition, 10, 20); got != tt.want {
				t.Errorf("Ternary(%t, 10, 20) = %d, want %d", tt.condition, got, tt.want)
			}
		})
	}

	t.Run("string values", func(t *testing.T) {
		t.Parallel()
		if got := Ternary(true, "parsed", "default"); got != "parsed" {
			t.Errorf("Ternary(true, parsed, default) = %q, want parsed", got)
		}
	})
}
