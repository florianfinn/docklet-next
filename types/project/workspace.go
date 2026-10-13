// Source: getarcaneapp/arcane/types/project/workspace.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package project

import workspacetypes "github.com/florianfinn/docklet-next/types/workspace"

const (
	FileOpCreateFile   = workspacetypes.FileOpCreateFile
	FileOpCreateFolder = workspacetypes.FileOpCreateFolder
	FileOpUpdateFile   = workspacetypes.FileOpUpdateFile
	FileOpRename       = workspacetypes.FileOpRename
	FileOpMove         = workspacetypes.FileOpMove
	FileOpDelete       = workspacetypes.FileOpDelete
)

type WorkspaceFileChange = workspacetypes.FileChange

type WorkspaceUpdateManifest struct {
	FileTreeRevision string                `json:"fileTreeRevision" binding:"required"`
	FileChanges      []WorkspaceFileChange `json:"fileChanges" binding:"required" minItems:"1" maxItems:"500"`
}

type CreateProjectWorkspaceManifest struct {
	FileChanges []WorkspaceFileChange `json:"fileChanges" binding:"required" maxItems:"500"`
}
