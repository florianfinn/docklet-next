// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original registration-bound origin overlay and conditional assignment storage.

package origin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	dto "github.com/florianfinn/docklet-next/types/container"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Override struct {
	RegistrationID string    `gorm:"primaryKey;not null"`
	ContainerName  string    `gorm:"primaryKey;not null"`
	DockerID       string    `gorm:"not null"`
	Origin         string    `gorm:"not null"`
	AssignedBy     string    `gorm:"not null"`
	AssignedAt     time.Time `gorm:"not null"`
}

func (Override) TableName() string { return "container_origin_overrides" }

type Identity struct {
	ID, Name  string
	Labels    map[string]string
	Protected bool
}
type Store struct {
	DB                *gorm.DB
	LocalRegistration string
	Invalidate        func(string)
}

func locked(value dto.Origin) error {
	return &dto.Problem{Code: "container.origin-locked", Status: 409, Origin: value}
}

// HasComposeLabel reports any Compose label. Partial Compose metadata keeps the
// container locked: it can become Unraid or stay unknown, never standalone.
func HasComposeLabel(labels map[string]string) bool {
	for key := range labels {
		if strings.HasPrefix(key, "com.docker.compose.") {
			return true
		}
	}
	return false
}

// Detect uses label presence for Unraid and incomplete Compose identities.
func Detect(labels map[string]string) dto.OriginFields {
	value := dto.OriginStandalone
	if labels["com.docker.compose.project"] != "" && labels["com.docker.compose.service"] != "" {
		value = dto.OriginCompose
	} else if _, ok := labels["net.unraid.docker.managed"]; ok {
		value = dto.OriginUnraid
	} else {
		for key := range labels {
			if strings.HasPrefix(key, "com.docker.compose.") || strings.HasPrefix(key, "com.docker.swarm.") || strings.HasPrefix(key, "io.kubernetes.") {
				value = dto.OriginUnknown
				break
			}
		}
	}
	return dto.OriginFields{Origin: value, OriginSource: dto.OriginDetected}
}
func (s *Store) Registration(environment string) string {
	if environment == "0" {
		return s.LocalRegistration
	}
	return environment
}
func (s *Store) Effective(ctx context.Context, registration string, id Identity) (dto.OriginFields, error) {
	detected := Detect(id.Labels)
	if s == nil || s.DB == nil {
		return detected, nil
	}
	var row Override
	err := s.DB.WithContext(ctx).Where("registration_id=? AND (container_name=? OR docker_id=?)", registration, strings.TrimPrefix(id.Name, "/"), id.ID).Order("container_name").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return detected, nil
	}
	if err != nil {
		return dto.OriginFields{}, err
	}
	if row.DockerID != id.ID || row.ContainerName != strings.TrimPrefix(id.Name, "/") {
		detected.OriginSource = dto.OriginUnconfirmed
		return detected, nil
	}
	return dto.OriginFields{Origin: dto.Origin(row.Origin), OriginSource: dto.OriginManual}, nil
}
func (s *Store) Guard(ctx context.Context, registration string, id Identity) error {
	fields, err := s.Effective(ctx, registration, id)
	if err != nil {
		return err
	}
	if id.Protected || fields.Origin != dto.OriginStandalone || fields.OriginSource == dto.OriginUnconfirmed {
		return locked(fields.Origin)
	}
	return nil
}
func (s *Store) current(tx *gorm.DB, registration string) error {
	if registration == s.LocalRegistration {
		return nil
	}
	var n int64
	if err := tx.Table("environments").Where("id=?", registration).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return locked(dto.OriginUnknown)
	}
	return nil
}
func (s *Store) Assign(ctx context.Context, environment string, id Identity, request dto.OriginAssignment, actor string) error {
	detected := Detect(id.Labels)
	hasProject := HasComposeLabel(id.Labels)
	if id.ID != request.DockerID || detected.Origin != dto.OriginUnknown || (request.Origin != dto.OriginUnraid && request.Origin != dto.OriginStandalone) || (request.Origin == dto.OriginStandalone && hasProject) {
		return locked(detected.Origin)
	}
	registration := s.Registration(environment)
	row := Override{RegistrationID: registration, ContainerName: strings.TrimPrefix(id.Name, "/"), DockerID: id.ID, Origin: string(request.Origin), AssignedBy: actor, AssignedAt: time.Now().UTC()}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.current(tx, registration); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "registration_id"}, {Name: "container_name"}}, DoUpdates: clause.AssignmentColumns([]string{"docker_id", "origin", "assigned_by", "assigned_at"})}).Create(&row).Error
	})
	if err == nil && s.Invalidate != nil {
		s.Invalidate(environment)
	}
	return err
}
func (s *Store) Reset(ctx context.Context, environment string, id Identity) error {
	registration := s.Registration(environment)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.current(tx, registration); err != nil {
			return err
		}
		return tx.Where("registration_id=? AND (container_name=? OR docker_id=?)", registration, strings.TrimPrefix(id.Name, "/"), id.ID).Delete(&Override{}).Error
	})
	if err == nil && s.Invalidate != nil {
		s.Invalidate(environment)
	}
	return err
}

// Move is called inside the same transaction as terminal receipt projection. A
// reset or removed registration wins: completion never inserts an assignment.
func Move(tx *gorm.DB, registration string, result *dto.RecreateResult) error {
	if result == nil {
		return nil
	}
	return tx.Model(&Override{}).Where("registration_id=? AND docker_id=? AND container_name=?", registration, result.OldID, result.OldName).Updates(map[string]any{"docker_id": result.NewID, "container_name": result.NewName}).Error
}

// Overlay preserves unknown response fields while visiting list, group and detail shapes.
func (s *Store) Overlay(ctx context.Context, environment string, body []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	var visit func(any) error
	visit = func(value any) error {
		switch v := value.(type) {
		case []any:
			for _, child := range v {
				if err := visit(child); err != nil {
					return err
				}
			}
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				if _, has := v["origin"]; has {
					name, _ := v["name"].(string)
					if names, ok := v["names"].([]any); ok && len(names) > 0 {
						name, _ = names[0].(string)
					}
					labels := map[string]string{}
					if raw, ok := v["labels"].(map[string]any); ok {
						for k, val := range raw {
							if str, ok := val.(string); ok {
								labels[k] = str
							}
						}
					}
					fields, err := s.Effective(ctx, s.Registration(environment), Identity{ID: id, Name: name, Labels: labels})
					if err != nil {
						return err
					}
					v["origin"], v["originSource"] = fields.Origin, fields.OriginSource
				}
			}
			for _, child := range v {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// AssignFresh snapshots the assignment before the host read and compares it in
// the write transaction. Reset, replacement projection and removal fence a stale read.
func (s *Store) AssignFresh(ctx context.Context, environment, containerID string, request dto.OriginAssignment, actor string, read func(context.Context, string, string) (Identity, error)) error {
	registration := s.Registration(environment)
	var before []Override
	if err := s.DB.WithContext(ctx).Where("registration_id=?", registration).Find(&before).Error; err != nil {
		return err
	}
	identity, err := read(ctx, environment, containerID)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(identity.Name, "/")
	var expected *Override
	for i := range before {
		if before[i].ContainerName == name {
			expected = &before[i]
			break
		}
	}
	detected := Detect(identity.Labels)
	hasProject := HasComposeLabel(identity.Labels)
	if identity.ID != request.DockerID || detected.Origin != dto.OriginUnknown || (request.Origin != dto.OriginUnraid && request.Origin != dto.OriginStandalone) || (request.Origin == dto.OriginStandalone && hasProject) {
		return locked(detected.Origin)
	}
	next := Override{RegistrationID: registration, ContainerName: name, DockerID: identity.ID, Origin: string(request.Origin), AssignedBy: actor, AssignedAt: time.Now().UTC()}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.current(tx, registration); err != nil {
			return err
		}
		if expected == nil {
			change := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&next)
			if change.Error != nil {
				return change.Error
			}
			if change.RowsAffected != 1 {
				return locked(detected.Origin)
			}
			return nil
		}
		change := tx.Model(&Override{}).Where("registration_id=? AND container_name=? AND docker_id=? AND origin=? AND assigned_by=? AND assigned_at=?", registration, name, expected.DockerID, expected.Origin, expected.AssignedBy, expected.AssignedAt).Updates(map[string]any{"docker_id": next.DockerID, "origin": next.Origin, "assigned_by": next.AssignedBy, "assigned_at": next.AssignedAt})
		if change.Error != nil {
			return change.Error
		}
		if change.RowsAffected != 1 {
			return locked(detected.Origin)
		}
		return nil
	})
	if err == nil && s.Invalidate != nil {
		s.Invalidate(environment)
	}
	return err
}
