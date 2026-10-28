// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original same-filesystem ownership transfer for asynchronous restore input.

package upload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/upload"
	"uuid"
)

// TakeRestoreInput atomically removes a complete session from the expiring
// namespace. The accepted worker owns the returned directory until settlement.
func (s *UploadService) TakeRestoreInput(ctx context.Context, uploadID, restoreID string) (string, error) {
	defer s.locks.Lock(uploadID)()
	if _, err := uuid.Parse(restoreID); err != nil {
		return "", fmt.Errorf("invalid restore ID")
	}
	session, err := s.loadMetaInternal(ctx, upload.KindVolumeBackup, uploadID)
	if err != nil {
		return "", err
	}
	if !session.Complete {
		return "", common.ErrUploadSessionIncomplete
	}
	if session.Size > dto.MaxVolumeUploadBytes {
		return "", dto.NewProblem(413, "volume.backup.upload-too-large", nil)
	}
	source := filepath.Join(s.root, uploadID, sessionDataFilename)
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if info.Size() != session.Size {
		return "", common.ErrUploadSessionIncomplete
	}
	owned := filepath.Join(s.root, "restore-inputs", restoreID)
	if err = os.MkdirAll(filepath.Dir(owned), 0700); err != nil {
		return "", err
	}
	if err = os.Mkdir(owned, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(owned, "input")
	if err = os.Rename(source, target); err != nil {
		_ = os.Remove(owned)
		return "", err
	}
	_ = os.RemoveAll(filepath.Join(s.root, uploadID))
	return target, nil
}
