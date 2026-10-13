// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/restore/handler_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package restore

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	uploadtypes "github.com/florianfinn/docklet-next/types/upload"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/upload"
)

func TestUploadAndRestoreReturnsNotFoundForUnknownSession(t *testing.T) {
	h := &Handler{service: &Service{}, uploadService: upload.NewUploadService(nil)}

	ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.Actor{ID: "u-1"})

	_, err := h.UploadAndRestore(ctx, &UploadAndRestoreInput{
		EnvironmentID: "0",
		VolumeName:    "vol-1",
		Body:          uploadtypes.ConsumeRequest{UploadID: "missing"},
	})

	require.Error(t, err)

	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusNotFound, statusErr.GetStatus())
}
