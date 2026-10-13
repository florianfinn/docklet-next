// Source: getarcaneapp/arcane/types/project/compose_cache.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package project

// ComposeCache stores values until their Compose configuration inputs change.
// Fingerprints identify the settings and project identity used when loading.
type ComposeCache[T any] interface {
	Get(projectID, fingerprint string) (T, bool)
	Set(projectID, fingerprint, projectPath, projectsDirectory, composePath string, composeFiles, envFiles []string, value T) error
	Invalidate(projectID string)
}

// ComposeDependencies records files consumed before executable models discard env_file.
type ComposeDependencies struct {
	EnvFiles []string
}
