// Source: docklet-next/issues/493
// Revision: 62850500fcd7feb2bd1c9071324945301c267e36
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original pull bookkeeping for tools image pulls outside PullImage.

package image

import (
	"context"

	usertypes "github.com/florianfinn/docklet-next/types/user"
)

// RecordHelperPull books a tools image pull made outside PullImage (volume
// helpers, proxy probe) like a system pull: audit event and registry usage.
func (s *ImageService) RecordHelperPull(ctx context.Context, ref string) {
	system := usertypes.SystemUser
	ctx = context.WithValue(ctx, usertypes.CurrentUserContextKey{}, &system)
	s.auditImageMutation(ctx, "pull", "", ref, nil)
	if s.registryService != nil {
		_ = s.registryService.RecordImagePull(ctx, ref)
	}
}
