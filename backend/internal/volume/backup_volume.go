// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original lazy, persistent backup volume selection shared by hub and agent.; M36 #570: Docklet backup volume and runtime names.

package volume

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/moby/moby/client"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DefaultBackupVolumeName = "docklet-backups"
const legacyBackupVolumeName = "arcane-backups"

// BackupVolumeResolver commits a choice only when named-volume storage is used.
// Metadata reads inspect the same choice without pinning it or creating storage.
type BackupVolumeResolver struct {
	mu       sync.Mutex
	explicit string
	resolved string
	read     func(context.Context) (string, error)
	write    func(context.Context, string) error
	inspect  func(context.Context, string) error
}

func (s *VolumeService) newBackupVolumeResolver(explicit string) *BackupVolumeResolver {
	resolver := &BackupVolumeResolver{
		explicit: strings.TrimSpace(explicit),
		read: func(ctx context.Context) (string, error) {
			var row settings.SettingVariable
			err := s.db.WithContext(ctx).Where("key = ?", "backupVolumeNameResolved").First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", nil
			}
			return strings.TrimSpace(row.Value), err
		},
		write: func(ctx context.Context, name string) error {
			row := settings.SettingVariable{Key: "backupVolumeNameResolved", Value: name}
			return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&row).Error
		},
		inspect: func(ctx context.Context, name string) error {
			cli, err := s.dockerService.GetClient(ctx)
			if err != nil {
				return err
			}
			_, err = cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
			return err
		},
	}
	if s.settingsService != nil {
		s.settingsService.SetBackupVolumeNameReader(resolver.Current)
	}
	return resolver
}

func (r *BackupVolumeResolver) choose(ctx context.Context) (string, error) {
	if r.resolved != "" {
		return r.resolved, nil
	}
	if r.explicit != "" {
		return r.explicit, nil
	}
	saved, err := r.read(ctx)
	if err != nil {
		return "", fmt.Errorf("read backup volume selection: %w", err)
	}
	if saved != "" {
		return saved, nil
	}
	err = r.inspect(ctx, legacyBackupVolumeName)
	switch {
	case err == nil:
		return legacyBackupVolumeName, nil
	case errdefs.IsNotFound(err):
		return DefaultBackupVolumeName, nil
	default:
		return "", fmt.Errorf("inspect legacy backup volume: %w", err)
	}
}

func (r *BackupVolumeResolver) Current(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.choose(ctx)
}

func (r *BackupVolumeResolver) Resolve(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resolved != "" {
		return r.resolved, nil
	}
	name, err := r.choose(ctx)
	if err != nil {
		return "", err
	}
	if err = r.write(ctx, name); err != nil {
		return "", fmt.Errorf("persist backup volume selection: %w", err)
	}
	r.resolved = name
	return name, nil
}

// Inventory protects both built-in names and a pinned custom name without inspecting Docker.
func (s *VolumeService) isBackupVolumeName(ctx context.Context, name string) bool {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, DefaultBackupVolumeName) || strings.EqualFold(name, legacyBackupVolumeName) {
		return true
	}
	selected := s.backupVolumeName
	if r := s.backupVolume; r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		switch {
		case r.explicit != "":
			selected = r.explicit
		case r.resolved != "":
			selected = r.resolved
		default:
			if s.db != nil {
				selected, _ = r.read(ctx)
			}
		}
	}
	return selected != "" && strings.EqualFold(name, strings.TrimSpace(selected))
}
