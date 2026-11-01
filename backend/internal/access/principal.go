// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/user"
	"gorm.io/gorm"
	"sync"
	"time"
)

type Assignment struct {
	ID            string `gorm:"primaryKey"`
	UserID        string
	EnvironmentID string
	ProjectID     string
	Level         string
	CreatedAt     time.Time
}

func (Assignment) TableName() string { return "user_app_assignments" }
func Assignments(ctx context.Context, db *gorm.DB, id string) ([]user.AppAssignment, error) {
	out := []user.AppAssignment{}
	err := db.WithContext(ctx).Table("user_app_assignments").Where("user_id = ?", id).Order("environment_id, project_id").Scan(&out).Error
	return out, err
}

// Synthesize discards legacy non-administrator roles. Only declared operations
// and their internal supporting grants reach the user's per-host permission set.
func Synthesize(id string, rows []user.AppAssignment) *authz.PermissionSet {
	ps := authz.NewPermissionSet()
	ps.UserID = id
	ps.Assignments = rows

	for _, a := range rows {
		grants := []string{"projects:list", "projects:read", "projects:deploy", "projects:down", "projects:restart", "projects:logs", "containers:read", "containers:start", "containers:stop", "containers:restart", "containers:logs", "containers:exec", "activities:read", "activities:cancel"}
		if a.Level == string(Full) {
			grants = append(grants, "projects:update", "projects:archive", "images:pull", "image-updates:read", "image-updates:check")
		}
		ps.AddEnv(a.EnvironmentID, grants...)
	}
	return ps
}

var revisions = struct {
	sync.Mutex
	values map[string]uint64
}{values: map[string]uint64{}}

func Revision(id string) uint64 {
	revisions.Lock()
	defer revisions.Unlock()
	return revisions.values[id]
}
func Revoke(id string) {
	revisions.Lock()
	revisions.values[id]++
	revisions.Unlock()
	sessionlife.Revoke("", id, "")
}

var ErrRevoked = errors.New("user rights revoked")
