// Source: getarcaneapp/arcane/backend/internal/image/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-24
// License: BSD-3-Clause
// Deviations: Separate ordinary Compose pulls from M10 image admission; select saved host auth only.

package image

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/docker"
	"io"
)

// PullComposeImage is the existing deployment port, outside M10 image admission.
// Compose runtime owns its diagnostics; credentials come from this executor's store.
func (s *ImageService) PullComposeImage(ctx context.Context, ref string, writer io.Writer, actor usertypes.Actor, _ []containerregistry.Credential) (resultErr error) {
	ctx = context.WithValue(ctx, usertypes.CurrentUserContextKey{}, &actor)
	defer func() { s.auditImageMutation(ctx, "pull", "", ref, resultErr) }()
	if err := guard.ValidateReference(ref); err != nil {
		return err
	}
	opts, err := s.PullOptionsWithAuth(ctx, ref, nil)
	if err != nil {
		return err
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	if s.eventService != nil {
		defer s.eventService.BeginDockerResourceSuppressionWindow("image", "", ref)()
	}
	reader, err := c.ImagePull(ctx, ref, opts)
	if err != nil {
		return err
	}
	defer reader.Close()
	lines := docker.NewLogLineWriter(writer)
	err = docker.RenderJSONMessageStream(reader, lines)
	_ = lines.Close()
	if err != nil {
		return err
	}
	if s.registryService != nil {
		_ = s.registryService.RecordImagePull(ctx, ref)
	}
	return nil
}
