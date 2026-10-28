// Source: getarcaneapp/arcane/backend/pkg/libarcane/rustic/image.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package rustic provides shared Rustic runtime configuration.
package rustic

import (
	"context"
	"strings"
)

const (
	// DefaultImage is the official Rustic container image used by backup and recovery flows.
	DefaultImage = "ghcr.io/rustic-rs/rustic:v0.11.2@sha256:fce5854a2df9fdaa1f5999df9d99d832aa8619c79eefb3e4136da46bf912a1b0"

	// CacheVolume persists Rustic's repository cache across ephemeral runtime containers.
	CacheVolume = "arcane-rustic-cache"
)

type imageKey struct{}

func WithImage(ctx context.Context, reference string) context.Context {
	return context.WithValue(ctx, imageKey{}, strings.TrimSpace(reference))
}
func Image(ctx context.Context) string {
	if value, ok := ctx.Value(imageKey{}).(string); ok && value != "" {
		return value
	}
	return DefaultImage
}
