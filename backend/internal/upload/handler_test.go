// Source: docklet-next/issues/284
// Revision: 30d80ec7904e6faeeea28a36914e9d5a30b9d2da
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original upload-session refusal contract test.

package upload

import (
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/upload"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func TestCreateSessionRefusesOversizedVolumeBackupWithTypedProblem(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uploads")
	handler := &UploadHandler{uploadService: &UploadService{root: root}}
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())

	_, err := handler.CreateSession(ctx, &CreateUploadSessionInput{
		EnvironmentID: "0",
		Kind:          upload.KindVolumeBackup,
		Body:          upload.CreateSessionRequest{Filename: "restore.tar.gz", Size: dto.MaxVolumeUploadBytes + 1},
	})
	var status huma.StatusError
	if !errors.As(err, &status) || status.GetStatus() != 413 {
		t.Fatalf("CreateSession error = %v, want HTTP 413", err)
	}
	body, marshalErr := json.Marshal(status)
	if marshalErr != nil {
		t.Fatalf("marshal refusal: %v", marshalErr)
	}
	var wire struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	if unmarshalErr := json.Unmarshal(body, &wire); unmarshalErr != nil {
		t.Fatalf("unmarshal refusal: %v", unmarshalErr)
	}
	if wire.Status != 413 || wire.Code != "volume.backup.upload-too-large" {
		t.Fatalf("refusal body = %s, want status 413 and code volume.backup.upload-too-large", body)
	}
	if entries, readErr := os.ReadDir(root); readErr == nil && len(entries) > 0 {
		t.Fatalf("oversized refusal staged %d session entries", len(entries))
	}
}
