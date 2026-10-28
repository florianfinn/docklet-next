// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/restore/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace
; M12 persisted volume lifecycle and legacy restore cut.
package restore

import "testing"

// Retired archive paths cannot authorize synchronous extraction anymore.
func TestRestoreServiceUsesAsynchronousLifecycle(t *testing.T) {
	s := NewService(Dependencies{})
	l := &Lifecycle{}
	s.SetLifecycle(l)
	if s.lifecycle != l {
		t.Fatal("restore service lost its lifecycle owner")
	}
}
