// Source: docklet-next/issues/434
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original destroy flag default tests.

package project

import "testing"

func TestDestroyFlagsKeepFilesUnlessExplicit(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name        string
		body        *Destroy
		files, vols bool
	}{
		{"no body", nil, false, false},
		{"empty", &Destroy{}, false, false},
		{"explicit false", &Destroy{RemoveFiles: &no, RemoveVolumes: true}, false, true},
		{"explicit true", &Destroy{RemoveFiles: &yes}, true, false},
	} {
		files, vols := tc.body.Flags()
		if files != tc.files || vols != tc.vols {
			t.Fatalf("%s: got (%t, %t), want (%t, %t)", tc.name, files, vols, tc.files, tc.vols)
		}
	}
}
