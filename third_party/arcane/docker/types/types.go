// Source: getarcaneapp/kit/docker/types/types.go
// Revision: e3d12f637964a6edd495f9b31a10d2f0a40fa127
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package types holds contracts shared between producers and consumers of the
// docker package.
package types

// ProgressWriterKey is the context key for an io.Writer that receives
// operation progress output, such as a writer wrapping an HTTP response.
type ProgressWriterKey struct{}
