//go:build !unix

// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Fail closed on targets without reviewed file alias checks.

package sqlitestore

import "os"

func hasMultipleLinks(os.FileInfo) bool { return true }
