// Source: getarcaneapp/arcane/types/volume/workspace.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; describe enforced optional content baselines.

package volume

import "github.com/florianfinn/docklet-next/types/workspace"

const (
	FileOpCreateFile   = workspace.FileOpCreateFile
	FileOpCreateFolder = workspace.FileOpCreateFolder
	FileOpUpdateFile   = workspace.FileOpUpdateFile
	FileOpRename       = workspace.FileOpRename
	FileOpMove         = workspace.FileOpMove
	FileOpDelete       = workspace.FileOpDelete
	FileOpRestoreFile  = "restore_file"
)

type WorkspaceFileChange struct {
	Operation     string `json:"operation" binding:"required" enum:"create_file,create_folder,update_file,rename,move,delete,restore_file"`
	RelativePath  string `json:"relativePath" binding:"required"`
	NewName       string `json:"newName,omitempty"`
	NewParentPath string `json:"newParentPath,omitempty"`
	UploadIndex   *int   `json:"uploadIndex,omitempty" minimum:"0"`
	// BaselineIndex references the content an update_file was drafted against.
	// When set, apply conflicts if the current file content no longer matches.
	BaselineIndex *int   `json:"baselineIndex,omitempty" minimum:"0"`
	BackupID      string `json:"backupId,omitempty"`
	Recursive     bool   `json:"recursive,omitempty"`
}

type WorkspaceUpdateManifest struct {
	FileTreeRevision string                `json:"fileTreeRevision" binding:"required"`
	FileChanges      []WorkspaceFileChange `json:"fileChanges" binding:"required" minItems:"1" maxItems:"500"`
}
