// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split image resolution; host process configuration selects the complete pinned Rustic reference.

package backup

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"io"
	"strings"
)

// SetImageReference is wired once, before workers start. The supplier reads
// this host process's environment configuration on each operation.
func (e *Engine) SetImageReference(reference func() string) { e.imageReference = reference }
func (e *Engine) imageContext(ctx context.Context) context.Context {
	if e != nil && e.imageReference != nil {
		if reference := strings.TrimSpace(e.imageReference()); reference != "" {
			return rustic.WithImage(ctx, reference)
		}
	}
	return ctx
}
func (e *Engine) EngineImage(ctx context.Context) string { return rustic.Image(e.imageContext(ctx)) }

func (e *Engine) ensureImageInternal(ctx context.Context, dockerClient *client.Client) error {
	if _, err := dockerClient.ImageInspect(ctx, rustic.Image(ctx)); err == nil {
		return nil
	}
	if e.imageService == nil {
		return errors.New("image service is unavailable")
	}
	if err := e.imageService.PullImage(ctx, rustic.Image(ctx), io.Discard, user.SystemUser, nil); err != nil {
		return fmt.Errorf("failed to pull official Rustic image: %w", err)
	}
	return nil
}
