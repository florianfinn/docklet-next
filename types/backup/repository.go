// Source: getarcaneapp/arcane/types/backup/repository.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package backup

// RepositoryObservation describes a read-only S3 metadata check.
type RepositoryObservation struct {
	Available         bool
	Reason            string
	SnapshotAvailable bool
}
