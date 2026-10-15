// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original direct octet-stream route for issue #32.

package backup

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

type uploadResultKey struct{}

func (h *Handler) streamUploadMiddleware(api huma.API) huma.Middlewares {
	return huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
		if h.service == nil {
			next(ctx)
			return
		}
		media, _, err := mime.ParseMediaType(ctx.Header("Content-Type"))
		if err != nil || media != "application/octet-stream" {
			_ = huma.WriteErr(api, ctx, http.StatusUnsupportedMediaType, "backup.unsupported_media_type")
			return
		}
		request := humaecho.Unwrap(ctx).Request()
		if request.ContentLength > backuptypes.MaxSystemArchiveBytes {
			_ = huma.WriteErr(api, ctx, http.StatusRequestEntityTooLarge, "backup.size_limit")
			return
		}
		reader := ctx.BodyReader()
		if reader == nil {
			_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "backup.invalid_archive")
			return
		}
		controller := http.NewResponseController(humaecho.Unwrap(ctx).Response())
		if controller.SetReadDeadline(time.Now().Add(archiveDeadline)) == nil {
			defer controller.SetReadDeadline(time.Time{})
		}
		actor := usertypes.Actor{}
		if current, ok := userdomain.CurrentUserFromContext(ctx.Context()); ok && current != nil {
			actor = *current.Actor()
		}
		operation, err := h.service.UploadArchive(ctx.Context(), actor, reader)
		if err != nil {
			status := http.StatusUnprocessableEntity
			switch {
			case errors.Is(err, errArchiveLimit):
				status = http.StatusRequestEntityTooLarge
			case errors.Is(err, ErrSystemBackupAlreadyRunning):
				status = http.StatusConflict
			case errors.Is(err, ErrSystemBackupUnavailable):
				status = http.StatusServiceUnavailable
			case errors.Is(err, errArchiveTimeout):
				status = http.StatusRequestTimeout
			}
			_ = huma.WriteErr(api, ctx, status, uploadErrorCode(err))
			return
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), uploadResultKey{}, operation)))
	}}
}
