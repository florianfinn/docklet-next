// Source: getarcaneapp/arcane/types/backup/file_browser.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package backup

// BackupFileEntry describes one file or directory under a backup's logical
// restore root.
type BackupFileEntry struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
}

// RestoreSelection selects explicit paths or every eligible backup entry.
type RestoreSelection struct {
	Paths     []string `json:"paths,omitempty"`
	SelectAll bool     `json:"selectAll,omitempty"`
	Search    string   `json:"search,omitempty"`
}
