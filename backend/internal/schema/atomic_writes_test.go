// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Real concurrent service writes against the full baseline and one-connection pool.

package schema

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func concurrentCalls(t *testing.T, n int, call func(int) error) []error {
	t.Helper()
	start := make(chan struct{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	failures := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; failures[i] = call(i) }()
	}
	close(start)
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("service writes did not drain")
	}
	return failures
}
func TestConcurrentKVIncrementsAndExactPrefixes(t *testing.T) {
	db := cascadeDatabase(t)
	svc := kv.NewKVService(db)
	for _, err := range concurrentCalls(t, 128, func(int) error { _, err := svc.IncrementInt64(t.Context(), "counter", 1); return err }) {
		require.NoError(t, err)
	}
	value, err := svc.GetInt64(t.Context(), "counter", 0)
	require.NoError(t, err)
	require.EqualValues(t, 128, value)
	for _, key := range []string{"Prefix_A", "prefix_A", `Prefix_%\value`, "PrefixX"} {
		require.NoError(t, svc.Set(t.Context(), key, "v"))
	}
	rows, err := svc.ListByPrefix(t.Context(), "Prefix_")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	rows, err = svc.ListByPrefix(t.Context(), `Prefix_%\`)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var names []string
	require.NoError(t, db.Raw("SELECT key FROM kv WHERE key LIKE 'prefix%' ORDER BY key").Scan(&names).Error)
	require.Equal(t, []string{"PrefixX", `Prefix_%\value`, "Prefix_A", "prefix_A"}, names)
}
func TestConcurrentMetadataAppendUpdateAndCompletion(t *testing.T) {
	db := cascadeDatabase(t)
	svc := activity.NewActivityService(db, nil)
	run, err := svc.StartActivity(t.Context(), activity.StartActivityRequest{Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	for _, err := range concurrentCalls(t, 24, func(i int) error {
		if i%3 == 0 {
			return svc.PatchActivityMetadata(t.Context(), run.ID, database.JSON{fmt.Sprintf("key-%d", i): i})
		}
		if i%3 == 1 {
			_, err := svc.AppendMessage(t.Context(), run.ID, activity.AppendActivityMessageRequest{Message: fmt.Sprintf("line-%d", i)})
			return err
		}
		_, err := svc.UpdateActivity(t.Context(), run.ID, activity.UpdateActivityRequest{Progress: new(i)})
		return err
	}) {
		require.NoError(t, err)
	}
	var stored activity.Activity
	require.NoError(t, db.First(&stored, "id=?", run.ID).Error)
	require.Len(t, stored.Metadata, 8)
	for _, err := range concurrentCalls(t, 12, func(int) error {
		_, err := svc.CompleteActivity(t.Context(), run.ID, activitytypes.StatusSuccess, "finished", nil)
		return err
	}) {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&activity.ActivityMessage{}).Where("activity_id=?", run.ID).Count(&count).Error)
	require.EqualValues(t, 9, count)
	_, err = svc.UpdateActivity(t.Context(), run.ID, activity.UpdateActivityRequest{Progress: new(1)})
	require.Error(t, err)
	require.Error(t, svc.PatchActivityMetadata(t.Context(), run.ID, database.JSON{"late": true}))
	require.NoError(t, db.First(&stored, "id=?", run.ID).Error)
	require.Equal(t, activitytypes.StatusSuccess, stored.Status)
	require.NotNil(t, stored.DurationMs)
	require.GreaterOrEqual(t, *stored.DurationMs, int64(0))
}
func TestConcurrentAdminAssignmentsAndRoleUniqueness(t *testing.T) {
	db := cascadeDatabase(t)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	ids := []string{"first", "second"}
	for _, id := range ids {
		_, err := users.CreateUser(t.Context(), &user.User{ID: id, Username: id, PasswordHash: "test-only"})
		require.NoError(t, err)
		require.NoError(t, roles.SetUserAssignments(t.Context(), id, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
	}
	failures := concurrentCalls(t, 2, func(i int) error { return roles.SetUserAssignments(t.Context(), ids[i], nil) })
	success, guarded := 0, 0
	for _, err := range failures {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, common.ErrNoGlobalAdminRemains)
			guarded++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, guarded)
	for _, id := range ids {
		_, err := roles.CreateRole(t.Context(), id, nil, []string{authz.PermUsersRead})
		require.NoError(t, err)
	}
	var records []role.Role
	require.NoError(t, db.Where("name IN ?", ids).Order("name").Find(&records).Error)
	require.Len(t, records, 2)
	failures = concurrentCalls(t, 2, func(i int) error {
		_, err := roles.UpdateRole(t.Context(), records[i].ID, "same-name", nil, []string{authz.PermUsersRead})
		return err
	})
	success = 0
	for _, err := range failures {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, common.ErrRoleNameTaken)
		}
	}
	require.Equal(t, 1, success)
	// A deleted target must not be resurrected by a concurrent user update.
	target, err := users.CreateUser(t.Context(), &user.User{ID: "target", Username: "target", PasswordHash: "test-only"})
	require.NoError(t, err)
	failures = concurrentCalls(t, 12, func(i int) error {
		if i == 0 {
			return users.DeleteUser(t.Context(), target.ID, nil)
		}
		copy := *target
		copy.DisplayName = new(fmt.Sprintf("name-%d", i))
		_, err := users.UpdateUser(context.Background(), &copy, nil)
		if errors.Is(err, common.ErrUserNotFound) {
			return nil
		}
		return err
	})
	for _, err := range failures {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&user.User{}).Where("id=?", target.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestConcurrentPasswordChangesRevokeSessionsAtomically(t *testing.T) {
	db := cascadeDatabase(t)
	users := user.NewUserService(db, nil, session.RevokeAllUserSessionsExceptInDB)
	target, err := users.CreateUser(t.Context(), &user.User{ID: "password-target", Username: "password-target", PasswordHash: "test-only"})
	require.NoError(t, err)
	now := time.Now()
	sessions := []session.UserSession{{UserID: target.ID, RefreshTokenHash: fmt.Sprint("test-only-", 1), LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}, {UserID: target.ID, RefreshTokenHash: fmt.Sprint("test-only-", 2), LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}}
	require.NoError(t, db.Create(&sessions).Error)
	passwords := []string{"first-synthetic-password", "second-synthetic-password"}
	for _, err := range concurrentCalls(t, 2, func(i int) error {
		copy := *target
		_, err := users.SetPasswordAndRevokeSessionsExcept(t.Context(), &copy, passwords[i], sessions[i].ID)
		return err
	}) {
		require.NoError(t, err)
	}
	var stored user.User
	require.NoError(t, db.First(&stored, "id=?", target.ID).Error)
	require.True(t, users.ValidatePassword(stored.PasswordHash, passwords[0]) == nil || users.ValidatePassword(stored.PasswordHash, passwords[1]) == nil)
	var active int64
	require.NoError(t, db.Model(&session.UserSession{}).Where("user_id=? AND revoked_at IS NULL", target.ID).Count(&active).Error)
	require.Zero(t, active)
}

func TestSQLiteConstraintBoundaries(t *testing.T) {
	db := cascadeDatabase(t)
	require.ErrorContains(t, db.Exec("INSERT INTO activity_sync_receipts(run_id,updated_at) VALUES(NULL,1)").Error, "NOT NULL constraint failed: activity_sync_receipts.run_id")
	require.NoError(t, db.Exec("INSERT INTO activities(id,created_at,environment_id,type,status,started_at,gate_scope,gate_key) VALUES('queued',1,'0','project_deploy','queued',1,'scope','op')").Error)
	require.Error(t, db.Exec("INSERT INTO activities(id,created_at,environment_id,type,status,started_at,gate_scope,gate_key) VALUES('contender',1,'0','project_deploy','running',1,'scope','op')").Error)
	require.Error(t, db.Exec("UPDATE activities SET metadata='invalid' WHERE id='queued'").Error)
	require.NoError(t, db.Exec("UPDATE activities SET metadata='null' WHERE id='queued'").Error)
	require.NoError(t, db.Exec("UPDATE activities SET metadata=NULL WHERE id='queued'").Error)
	require.Error(t, db.Exec("UPDATE activities SET status='unknown' WHERE id='queued'").Error)
	require.Error(t, db.Exec("UPDATE activities SET gate_key='' WHERE id='queued'").Error)
	actor := user.User{Username: "scope-user", PasswordHash: "test-only"}
	require.NoError(t, db.Create(&actor).Error)
	localRole := role.Role{Name: "scope-role", Permissions: database.StringSlice{authz.PermUsersRead}}
	require.NoError(t, db.Create(&localRole).Error)
	assignment := role.UserRoleAssignment{UserID: actor.ID, RoleID: localRole.ID}
	require.NoError(t, db.Create(&assignment).Error)
	duplicate := role.UserRoleAssignment{UserID: actor.ID, RoleID: localRole.ID}
	require.Error(t, db.Create(&duplicate).Error)
	key := apikey.ApiKey{Name: "scope-key", KeyHash: "scope-hash", KeyPrefix: "scope"}
	require.NoError(t, db.Create(&key).Error)
	permission := role.ApiKeyPermission{ApiKeyID: key.ID, Permission: authz.PermUsersRead}
	require.NoError(t, db.Create(&permission).Error)
	duplicatePermission := role.ApiKeyPermission{ApiKeyID: key.ID, Permission: authz.PermUsersRead}
	require.Error(t, db.Create(&duplicatePermission).Error)
	require.NoError(t, db.Exec("UPDATE users SET preferences='{}' WHERE id=?", actor.ID).Error)
	require.Error(t, db.Exec("UPDATE users SET preferences='invalid' WHERE id=?", actor.ID).Error)
}

func TestSQLiteNumericAllocationAndTimeDefaults(t *testing.T) {
	db := cascadeDatabase(t)
	require.NoError(t, db.Exec("INSERT INTO notification_settings(provider,config) VALUES('example','{}')").Error)
	var first int64
	require.NoError(t, db.Raw("SELECT id FROM notification_settings").Scan(&first).Error)
	require.Positive(t, first)
	var kind string
	require.NoError(t, db.Raw("SELECT typeof(created_at) FROM notification_settings").Scan(&kind).Error)
	require.Equal(t, "integer", kind)
	require.NoError(t, db.Exec("DELETE FROM notification_settings").Error)
	require.NoError(t, db.Exec("INSERT INTO notification_settings(provider,config) VALUES('example','{}')").Error)
	var next int64
	require.NoError(t, db.Raw("SELECT id FROM notification_settings").Scan(&next).Error)
	require.Greater(t, next, first)
}
