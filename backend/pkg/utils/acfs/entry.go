// Source: getarcaneapp/arcane/backend/pkg/utils/acfs/entry.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package acfs maps ACFS protocol types to Arcane workspace contracts.
package acfs

import (
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/types/workspace"
	acfstypes "go.getarcane.app/acfs/types"
)

// FileEntry converts ACFS metadata to Arcane's shared workspace entry type.
func FileEntry(entry acfstypes.Entry) workspace.FileEntry {
	modTime := entry.ModTime
	if entry.ModTimeUnixNano != 0 {
		modTime = time.Unix(0, entry.ModTimeUnixNano)
	}

	return workspace.FileEntry{
		ModTime:      modTime,
		Name:         entry.Name,
		Path:         entry.Path,
		RelativePath: entry.Path,
		Mode:         entry.Mode,
		LinkTarget:   entry.LinkTarget,
		Size:         entry.Size,
		IsDirectory:  entry.IsDirectory,
		IsSymlink:    entry.IsSymlink,
	}
}

// IsRegular reports whether an ACFS entry describes a regular file.
func IsRegular(entry acfstypes.Entry) bool {
	return !entry.IsDirectory && !entry.IsSymlink && strings.HasPrefix(entry.Mode, "-")
}
