// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Real migrated SQLite revisions, transaction rollback and restart dedup evidence.

package policies

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
)

type storeFixture struct {
	db *database.DB
	Store
	url string
}

func newStoreFixture(t *testing.T) *storeFixture {
	t.Helper()
	f := &storeFixture{url: sqlitestore.URL(filepath.Join(t.TempDir(), "policy.db"))}
	var err error
	f.db, err = database.InitializeHub(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	f.Store = Store{DB: f.db.DB}
	t.Cleanup(func() { require.NoError(t, f.db.Close()) })
	return f
}
func (f *storeFixture) reopen(t *testing.T) {
	t.Helper()
	require.NoError(t, f.db.Close())
	var err error
	f.db, err = database.InitializeHub(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	f.Store.DB = f.db.DB
}
func (f *storeFixture) policy(t *testing.T, mode string) Policy {
	t.Helper()
	p, err := f.Policy(t.Context(), "0", "example-registration", "example-stack")
	require.NoError(t, err)
	p.Mode = mode
	p.ActorID = "example-actor"
	p.TargetIdentity = strings.Repeat("a", 64)
	require.NoError(t, f.SavePolicy(t.Context(), p, p.Revision))
	p.Revision++
	return p
}
func TestPolicyDefaultsAndRevisionCAS(t *testing.T) {
	f := newStoreFixture(t)
	p, err := f.Policy(t.Context(), "0", "registration", "stack")
	require.NoError(t, err)
	require.Equal(t, "off", p.Mode)
	require.Zero(t, p.Revision)
	p.Mode = "notify"
	p.ActorID = "actor"
	p.TargetIdentity = strings.Repeat("a", 64)
	require.NoError(t, f.SavePolicy(t.Context(), p, 0))
	p.Mode = "apply"
	err = f.SavePolicy(t.Context(), p, 0)
	require.Equal(t, "update.policy-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	got, err := f.Policy(t.Context(), "0", "registration", "stack")
	require.NoError(t, err)
	require.Equal(t, "notify", got.Mode)
	require.EqualValues(t, 1, got.Revision)
	p.Mode = "container"
	err = f.SavePolicy(t.Context(), p, 1)
	require.Equal(t, "update.invalid-policy", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	other, err := f.Policy(t.Context(), "0", "replacement-registration", "stack")
	require.NoError(t, err)
	require.Equal(t, "off", other.Mode)
	f.reopen(t)
	got, err = f.Policy(t.Context(), "0", "registration", "stack")
	require.NoError(t, err)
	require.EqualValues(t, 1, got.Revision)
}
func TestCandidateEntryDedupPersistsAcrossRestartAndPolicyToggle(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "notify")
	c := dto.UpdateCandidate{Service: "app", ImageRef: "registry.example.com/app:1", Version: "1.1.0", Digest: "sha256:" + strings.Repeat("b", 64), Platform: "linux/amd64"}
	first, err := f.RecordCandidate(t.Context(), p, c, true)
	require.NoError(t, err)
	require.NotEmpty(t, first.EntryID)
	require.NoError(t, f.db.Model(&Notice{}).Where("id = ?", first.EntryID).Update("status", "failed").Error)
	f.reopen(t)
	p.Mode = "off"
	require.NoError(t, f.SavePolicy(t.Context(), p, p.Revision))
	p.Revision++
	p.Mode = "notify"
	require.NoError(t, f.SavePolicy(t.Context(), p, p.Revision))
	p.Revision++
	for range 3 {
		repeated, e := f.RecordCandidate(t.Context(), p, c, true)
		require.NoError(t, e)
		require.Equal(t, first.EntryID, repeated.EntryID)
		require.Equal(t, "failed", repeated.NotificationState)
	}
	var count int64
	require.NoError(t, f.db.Model(&Candidate{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, f.db.Model(&Notice{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	c.Digest = "sha256:" + strings.Repeat("c", 64)
	next, err := f.RecordCandidate(t.Context(), p, c, true)
	require.NoError(t, err)
	require.NotEqual(t, first.EntryID, next.EntryID)
	require.NoError(t, f.db.Model(&Notice{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestCandidateAndEntryCommitAtomically(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "notify")
	require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_notice_failure BEFORE INSERT ON image_update_notifications BEGIN SELECT RAISE(ABORT,'synthetic notice failure'); END").Error)
	_, err := f.RecordCandidate(t.Context(), p, dto.UpdateCandidate{ImageRef: "registry.example.com/app:1", Digest: "sha256:" + strings.Repeat("b", 64), Platform: "linux/amd64"}, true)
	require.Error(t, err)
	for _, model := range []any{&Candidate{}, &Notice{}} {
		var count int64
		require.NoError(t, f.db.Model(model).Count(&count).Error)
		require.Zero(t, count, "failed entry insert cannot consume the dedup identity")
	}
}
func TestCandidateRejectsPolicyChangedDuringCheck(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "notify")
	changed := p
	changed.Mode = "off"
	require.NoError(t, f.SavePolicy(t.Context(), changed, p.Revision))
	_, err := f.RecordCandidate(t.Context(), p, dto.UpdateCandidate{}, true)
	require.Equal(t, "update.policy-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	var count int64
	require.NoError(t, f.db.Model(&Notice{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestChildCheckpointIsIdempotentAndPolicyRevisionIsAdmissionBound(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "apply")
	first, fresh, err := f.PrepareChild(t.Context(), p, "example-job")
	require.NoError(t, err)
	require.True(t, fresh)
	duplicate, fresh, err := f.PrepareChild(t.Context(), p, "example-job")
	require.NoError(t, err)
	require.False(t, fresh)
	require.Equal(t, first.ID, duplicate.ID)
	binding := dto.PolicyBinding{PolicyID: p.ID, Revision: p.Revision, JobRunID: first.JobRunID, ChildID: first.ID}
	require.NoError(t, ValidateAdmission(f.db.DB, &binding, p.EnvironmentID, p.RegistrationID, p.TargetID, p.TargetIdentity, p.ActorID))
	stale := binding
	stale.Revision++
	require.Equal(t, "update.policy-changed", imageDTO.Neutral(ValidateAdmission(f.db.DB, &stale, p.EnvironmentID, p.RegistrationID, p.TargetID, p.TargetIdentity, p.ActorID), "unexpected", 500).Problem.Code)
	require.NoError(t, AdmitChild(f.db.DB, &binding, first.ID))
	require.Error(t, AdmitChild(f.db.DB, &binding, first.ID), "the prepared checkpoint is consumed exactly once")
	require.Error(t, ValidateAdmission(f.db.DB, &binding, p.EnvironmentID, p.RegistrationID, p.TargetID, p.TargetIdentity, p.ActorID))
}

func TestCandidateIdentityUsesRepositoryWithoutSourceTag(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "notify")
	candidate := dto.UpdateCandidate{ImageRef: "registry.example.com/app:1.0.0", Version: "1.1.0", Digest: "sha256:" + strings.Repeat("a", 64), Platform: "linux/amd64"}
	first, err := f.RecordCandidate(t.Context(), p, candidate, true)
	require.NoError(t, err)
	candidate.ImageRef = "registry.example.com/app:1.1.0"
	second, err := f.RecordCandidate(t.Context(), p, candidate, true)
	require.NoError(t, err)
	require.Equal(t, first.EntryID, second.EntryID)
}
