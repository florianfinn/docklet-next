// Source: getarcaneapp/arcane/backend/pkg/utils/perms.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package utils

import "os"

var (
	FilePerm os.FileMode = 0o644
	DirPerm  os.FileMode = 0o755
)
