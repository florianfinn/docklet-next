// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original hub target CRUD, encrypted keys and retire/admission fence.

package backuptarget

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
	"uuid"

	backup "github.com/florianfinn/docklet-next/backend/internal/backup/keyformat"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

type Dependencies struct {
	Store      *Store
	References func(context.Context, Target) (dto.TargetReferences, []string, error)
	Sync       func(context.Context) error
}

type Service struct {
	deps          Dependencies
	mu            sync.Mutex
	fences        map[string]*sync.RWMutex
	refreshChecks func(context.Context) error
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps, fences: map[string]*sync.RWMutex{}}
}
func (s *Service) fence(id string) *sync.RWMutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fences[id] == nil {
		s.fences[id] = &sync.RWMutex{}
	}
	return s.fences[id]
}

// WithActive holds retirement off until a policy/manual admission response. The
// caller must not enter this method while already holding this target's fence.
func (s *Service) WithActive(ctx context.Context, id string, run func() error) error {
	lock := s.fence(id)
	lock.RLock()
	defer lock.RUnlock()
	if id != "local" {
		var row Target
		if err := s.deps.Store.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
			return dto.NewProblem(422, "backup.target.unknown", err)
		}
		if row.RetiredAt != nil {
			return dto.NewProblem(422, "backup.target.retired", nil)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return run()
}
func bump(tx *gorm.DB) (int64, error) {
	text, err := readSetting(tx, "backup_target_set_version")
	if err != nil {
		return 0, err
	}
	version, err := strconv.ParseInt(defaultString(text, "0"), 10, 64)
	if err != nil {
		return 0, err
	}
	if version == int64(^uint64(0)>>1) {
		return 0, invalid()
	}
	version++
	return version, writeSetting(tx, "backup_target_set_version", strconv.FormatInt(version, 10))
}
func (s *Service) Create(ctx context.Context, input dto.TargetInput) (Target, error) {
	input, err := Validate(input)
	if err != nil {
		return Target{}, err
	}
	if _, _, err = s.deps.Store.Authority(ctx); err != nil {
		return Target{}, err
	}
	key := input.RecoveryKey
	if key == "" {
		key, err = backup.GenerateRecoveryKey()
		if err != nil {
			return Target{}, err
		}
	}
	keyEnc, err := crypto.Encrypt(key)
	if err != nil {
		return Target{}, err
	}
	secret := ""
	if input.Secret != nil {
		secret = *input.Secret
	}
	secretEnc, err := encryptOptional(secret)
	if err != nil {
		return Target{}, err
	}
	row := Target{ID: uuid.New().String(), Name: input.Name, Kind: input.Kind, HostID: input.HostID, Path: input.Path, Server: input.Server, Share: input.Share, Username: input.Username, Options: input.Options, SecretEnc: secretEnc, RecoveryKeyEnc: keyEnc, CreatedAt: time.Now().UTC()}
	err = s.deps.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := uniqueTarget(tx, row, ""); e != nil {
			return e
		}
		version, e := bump(tx)
		if e != nil {
			return e
		}
		row.ConfigVersion = version
		return tx.Create(&row).Error
	})
	if err == nil && s.deps.Sync != nil {
		err = s.deps.Sync(ctx)
	}
	return row, err
}
func uniqueTarget(tx *gorm.DB, row Target, exclude string) error {
	var rows []Target
	if err := tx.Where("retired_at IS NULL AND id <> ?", exclude).Find(&rows).Error; err != nil {
		return err
	}
	for _, old := range rows {
		if old.Name == row.Name {
			return dto.NewProblem(422, "backup.target.name-exists", nil)
		}
		if sameLocation(old.location(), row.location()) {
			return dto.NewProblem(422, "backup.target.duplicate", nil)
		}
	}
	return nil
}
func (s *Service) Update(ctx context.Context, id string, input dto.TargetInput) (Target, error) {
	input, err := Validate(input)
	if err != nil {
		return Target{}, err
	}
	lock := s.fence(id)
	lock.Lock()
	defer lock.Unlock()
	var row Target
	err = s.deps.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		if row.RetiredAt != nil {
			return dto.NewProblem(422, "backup.target.retired", nil)
		}
		if !sameLocation(row.location(), input.TargetLocation) {
			return dto.NewProblem(422, "backup.target.immutable", nil)
		}
		if input.RecoveryKey != "" {
			return dto.NewProblem(422, "backup.target.immutable", nil)
		}
		row.Name, row.Username, row.Options = input.Name, input.Username, input.Options
		if err := uniqueTarget(tx, row, id); err != nil {
			return err
		}
		if input.Secret != nil {
			encrypted, e := encryptOptional(*input.Secret)
			if e != nil {
				return e
			}
			row.SecretEnc = encrypted
		}
		version, e := bump(tx)
		if e != nil {
			return e
		}
		row.ConfigVersion = version
		now := time.Now().UTC()
		row.UpdatedAt = &now
		return tx.Save(&row).Error
	})
	if err == nil && s.deps.Sync != nil {
		err = s.deps.Sync(ctx)
	}
	return row, err
}
func (s *Service) Retire(ctx context.Context, id string) error {
	lock := s.fence(id)
	lock.Lock()
	defer lock.Unlock()
	var row Target
	if err := s.deps.Store.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return err
	}
	if row.RetiredAt != nil {
		return nil
	}
	if s.deps.References == nil {
		return dto.NewProblem(409, "backup.target.hosts-unknown", nil)
	}
	refs, unknown, err := s.deps.References(ctx, row)
	if err != nil {
		return dto.NewProblem(409, "backup.target.hosts-unknown", err)
	}
	if len(unknown) > 0 {
		e := dto.NewProblem(409, "backup.target.hosts-unknown", nil)
		e.Hosts = unknown
		return e
	}
	if len(refs.Policies)+len(refs.Backups)+len(refs.Running) > 0 {
		e := dto.NewProblem(409, "backup.target.in-use", nil)
		e.Volumes = append(append(refs.Policies, refs.Backups...), refs.Running...)
		return e
	}
	err = s.deps.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := bump(tx); e != nil {
			return e
		}
		return tx.Model(&Target{}).Where("id = ?", id).Update("retired_at", time.Now().UTC()).Error
	})
	if err == nil && s.deps.Sync != nil {
		err = s.deps.Sync(ctx)
	}
	return err
}
func (s *Service) List(ctx context.Context) ([]dto.Target, error) {
	if s.refreshChecks != nil {
		if err := s.refreshChecks(ctx); err != nil {
			return nil, err
		}
	}
	var rows []Target
	if err := s.deps.Store.DB.WithContext(ctx).Order("name,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]dto.Target, 0, len(rows))
	for _, row := range rows {
		checks := []Check{}
		if err := s.deps.Store.DB.WithContext(ctx).Where("target_id = ?", row.ID).Order("host_id").Find(&checks).Error; err != nil {
			return nil, err
		}
		result = append(result, project(row, checks))
	}
	return result, nil
}
func project(row Target, checks []Check) dto.Target {
	result := dto.Target{TargetLocation: row.location(), ID: row.ID, Name: row.Name, Username: row.Username, Options: row.Options, HasSecret: row.SecretEnc != "", ConfigVersion: row.ConfigVersion, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, RetiredAt: row.RetiredAt, Checks: []dto.TargetCheck{}}
	for _, check := range checks {
		result.Checks = append(result.Checks, dto.TargetCheck{HostID: check.HostID, SetVersion: check.SetVersion, ConfigVersion: check.ConfigVersion, ActivityID: check.ActivityID, OK: check.OK, Code: check.Code, FreeBytes: check.FreeBytes, TotalBytes: check.TotalBytes, SnapshotCount: check.SnapshotCount, At: check.At, EarlierSettings: check.ConfigVersion < row.ConfigVersion})
	}
	return result
}
func (s *Service) Get(ctx context.Context, id string) (dto.Target, error) {
	rows, err := s.List(ctx)
	if err != nil {
		return dto.Target{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return row, nil
		}
	}
	return dto.Target{}, gorm.ErrRecordNotFound
}
func (s *Service) RecoveryKey(ctx context.Context, id string) (string, error) {
	var row Target
	if err := s.deps.Store.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return "", err
	}
	value, err := crypto.Decrypt(row.RecoveryKeyEnc)
	if err != nil {
		return "", err
	}
	if err = backup.ValidateRecoveryKey(value); err != nil {
		return "", errors.New("invalid stored recovery key")
	}
	return value, nil
}

// ConfigureDelivery runs during route construction, before serving requests.
// It preserves the service's per-target fences across callback wiring.
func (s *Service) ConfigureDelivery(references func(context.Context, Target) (dto.TargetReferences, []string, error), sync func(context.Context) error) {
	s.deps.References = references
	s.deps.Sync = sync
}

// ConfigureCheckRefresh installs the metadata-only remote result reader before serving.
func (s *Service) ConfigureCheckRefresh(refresh func(context.Context) error) {
	s.refreshChecks = refresh
}
