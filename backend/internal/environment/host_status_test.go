// Source: docklet-next/issues/372
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original tests for the database host verdict rule and the owned status writes.

package environment

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const hostStatusRuleMigration = "migrations/sqlite/004_host_status_verdict.sql"

// hostStatusRuleStatements returns the trigger statements of the real embedded migration,
// so unit fixtures built with AutoMigrate carry the same rule as migrated databases.
func hostStatusRuleStatements(t *testing.T) []string {
	t.Helper()
	data, err := resources.FS.ReadFile(hostStatusRuleMigration)
	require.NoError(t, err)
	var statements []string
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "CREATE TRIGGER") {
			statements = append(statements, strings.TrimSpace(line))
		}
	}
	require.Len(t, statements, 3)
	return statements
}

func installHostStatusRuleForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, statement := range hostStatusRuleStatements(t) {
		require.NoError(t, db.Exec(statement).Error)
	}
}

func migratedHostDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

type hostRow struct {
	Status              string
	CredentialRevision  int64
	StatusCheckRevision int64
}

func readHostRow(t *testing.T, db *gorm.DB, id string) hostRow {
	t.Helper()
	var row hostRow
	require.NoError(t, db.Raw("SELECT status, credential_revision, status_check_revision FROM environments WHERE id = ?", id).Row().Scan(&row.Status, &row.CredentialRevision, &row.StatusCheckRevision))
	return row
}

func seedHost(t *testing.T, db *gorm.DB, id, status string, checkRevision int64) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO environments (id, created_at, name, api_url, status, enabled, is_edge, credential_revision, status_check_revision) VALUES (?, CURRENT_TIMESTAMP, ?, ?, ?, 1, 0, 0, ?)", id, id, "http://"+id+".example.com", status, checkRevision).Error)
}

// Row 1a: every write form that would replace the verdict without a check or credential
// revision is refused by the database, including a multi-row statement, which changes nothing.
func TestHostVerdictRuleRefusesBypassForms(t *testing.T) {
	db := migratedHostDB(t)
	sqlDB, err := db.SQLDB()
	require.NoError(t, err)
	forms := []struct {
		name  string
		write func(id string) error
	}{
		{"raw exec", func(id string) error {
			return db.Exec("UPDATE environments SET status = 'online' WHERE id = ?", id).Error
		}},
		{"update column", func(id string) error {
			return db.Model(&Environment{}).Where("id = ?", id).Update("status", "online").Error
		}},
		{"map with upper-case key", func(id string) error {
			return db.Model(&Environment{}).Where("id = ?", id).Updates(map[string]any{"STATUS": "offline"}).Error
		}},
		{"table update", func(id string) error {
			return db.Table("environments").Where("id = ?", id).Update("status", "pending").Error
		}},
		{"database/sql", func(id string) error {
			_, err := sqlDB.ExecContext(t.Context(), "UPDATE environments SET status = 'offline' WHERE id = ?", id)
			return err
		}},
		{"upsert", func(id string) error {
			return db.Exec("INSERT INTO environments (id, created_at, api_url, status) VALUES (?, CURRENT_TIMESTAMP, 'http://upsert.example.com', 'online') ON CONFLICT(id) DO UPDATE SET status = excluded.status", id).Error
		}},
		{"replace", func(id string) error {
			return db.Exec("REPLACE INTO environments (id, created_at, api_url, status) VALUES (?, CURRENT_TIMESTAMP, 'http://replace.example.com', 'online')", id).Error
		}},
	}
	// Re-insertion forms are ignored rather than refused, so INSERT OR IGNORE keeps working.
	ignored := map[string]bool{"upsert": true, "replace": true}
	for _, form := range forms {
		id := "verdict-" + strings.ReplaceAll(form.name, " ", "-")
		seedHost(t, db.DB, id, "update-required", 2)
		err := form.write(id)
		if ignored[form.name] {
			require.NoError(t, err, form.name)
		} else {
			require.ErrorContains(t, err, "host.status-verdict", form.name)
		}
		require.Equal(t, hostRow{"update-required", 0, 2}, readHostRow(t, db.DB, id), form.name)
	}
	seedHost(t, db.DB, "multi-protected", "update-required", 0)
	seedHost(t, db.DB, "multi-ordinary", "online", 0)
	err = db.Exec("UPDATE environments SET status = 'offline' WHERE id IN ('multi-protected', 'multi-ordinary')").Error
	require.ErrorContains(t, err, "host.status-verdict")
	require.Equal(t, "update-required", readHostRow(t, db.DB, "multi-protected").Status)
	require.Equal(t, "online", readHostRow(t, db.DB, "multi-ordinary").Status, "a refused statement changes no row")
}

// Row 1b: two stale all-column saves cannot first roll the check revision back and then
// "advance" it to clear a newer verdict.
func TestHostVerdictRuleRefusesStaleRevisionRollback(t *testing.T) {
	db := migratedHostDB(t)
	seedHost(t, db.DB, "stale", "update-required", 5)
	var stale Environment
	require.NoError(t, db.First(&stale, "id = ?", "stale").Error)
	stale.StatusCheckRevision = 3
	require.ErrorContains(t, db.Save(&stale).Error, "host.status-verdict")
	stale.Status = "online"
	stale.StatusCheckRevision = 4
	require.ErrorContains(t, db.Save(&stale).Error, "host.status-verdict")
	require.Equal(t, hostRow{"update-required", 0, 5}, readHostRow(t, db.DB, "stale"))
}

// Row 2: the transitions that may replace the verdict do so, the others keep it, and the
// check revision moves only with compatibility results.
func TestHostStatusOwnedTransitions(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	db := f.service.db.DB
	require.NoError(t, db.Exec("UPDATE environments SET status = 'update-required' WHERE id = ?", f.id).Error)
	_, err := f.service.CheckAgentCompatibility(t.Context(), f.id)
	require.NoError(t, err)
	require.Equal(t, hostRow{"online", 0, 1}, readHostRow(t, db, f.id), "compatibility result clears the verdict with a check revision")

	f.info.AgentContractFingerprint = "different"
	_, err = f.service.CheckAgentCompatibility(t.Context(), f.id)
	require.ErrorIs(t, err, ErrAgentUpdateRequired)
	require.Equal(t, hostRow{"update-required", 0, 2}, readHostRow(t, db, f.id))

	require.NoError(t, f.service.UpdateEnvironmentConnectionState(t.Context(), f.id, true))
	require.NoError(t, f.service.UpdateEnvironmentConnectionState(t.Context(), f.id, false))
	require.NoError(t, f.service.ReconcileEdgeStatusesOnStartup(t.Context()))
	require.Equal(t, hostRow{"update-required", 0, 2}, readHostRow(t, db, f.id), "transport and startup writes keep the verdict")

	require.NoError(t, f.service.linkEnvironmentApiKey(t.Context(), f.id, "replacement", "replacement-token", "", "", "Example"))
	require.Equal(t, hostRow{"pending", 1, 2}, readHostRow(t, db, f.id), "a registration transition clears the verdict with a credential revision")

	require.NoError(t, db.Exec("UPDATE environments SET status = 'update-required' WHERE id = ?", f.id).Error)
	_, err = f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{"status_check_revision": 9, "name": "Renamed"}, nil, nil)
	require.Error(t, err, "the generic edit cannot write the check revision")
	require.Equal(t, hostRow{"update-required", 1, 2}, readHostRow(t, db, f.id))
}

// Row 3: a compatibility result for a registration whose endpoint moved during the probe
// changes neither the stored verdict nor the check revision.
func TestHostStatusCompatibilityResultIsCredentialFenced(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	db := f.service.db.DB
	require.NoError(t, db.Exec("UPDATE environments SET status = 'update-required' WHERE id = ?", f.id).Error)
	// The last probe answer is the latest point before the write; earlier probes re-resolve the target.
	f.beforeRespond = func(method, path string) {
		if path == "/api/environments/0/system/health" {
			require.NoError(t, db.Exec("UPDATE environments SET api_url = 'http://moved.example.com', credential_revision = credential_revision + 1 WHERE id = ?", f.id).Error)
		}
	}
	_, err := f.service.CheckAgentCompatibility(t.Context(), f.id)
	require.ErrorIs(t, err, ErrInvalidEnvironmentAccessToken)
	require.Equal(t, hostRow{"update-required", 1, 0}, readHostRow(t, db, f.id))
}

// Row 6: a refused write leaves the host snapshot cache untouched.
func TestHostStatusRefusedWriteKeepsCache(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	require.NoError(t, f.service.db.Exec("UPDATE environments SET status = 'update-required' WHERE id = ?", f.id).Error)
	env, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	f.service.remoteEnvs.put(*env)
	outcome, err := f.service.writeHostStatusInternal(t.Context(), hostStatusWrite{Transition: statusProbe, ID: f.id, Updates: map[string]any{"status": "online"}})
	require.ErrorIs(t, err, ErrHostStatusVerdict)
	require.Equal(t, hostStatusRejected, outcome)
	cached, ok := f.service.remoteEnvs.get(f.id).Get()
	require.True(t, ok)
	require.Equal(t, "update-required", cached.Status)
}

// Row 7: a failed Direct probe keeps update-required (Q-372-PROBE b) through the owned
// write's explicit no-op, and still marks a reachable-then-failing online host offline.
func TestDirectProbeFailureKeepsVerdict(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	var outcomes []hostStatusOutcome
	f.service.statusWriteObserver = func(transition hostStatusTransition, outcome hostStatusOutcome) {
		if transition == statusProbe {
			outcomes = append(outcomes, outcome)
		}
	}
	f.mu.Lock()
	f.failPath = "/api/health"
	f.mu.Unlock()
	db := f.service.db.DB
	require.NoError(t, db.Exec("UPDATE environments SET status = 'update-required' WHERE id = ?", f.id).Error)
	status, err := f.service.TestConnection(t.Context(), f.id, nil)
	require.Error(t, err)
	require.Equal(t, "offline", status)
	require.Equal(t, []hostStatusOutcome{hostStatusNoop}, outcomes)
	require.Equal(t, "update-required", readHostRow(t, db, f.id).Status)

	require.NoError(t, db.Exec("UPDATE environments SET status = 'online', status_check_revision = status_check_revision + 1 WHERE id = ?", f.id).Error)
	_, err = f.service.TestConnection(t.Context(), f.id, nil)
	require.Error(t, err)
	require.Equal(t, []hostStatusOutcome{hostStatusNoop, hostStatusApplied}, outcomes)
	require.Equal(t, "offline", readHostRow(t, db, f.id).Status)
}

// Row 5: the unit fixture carries the real rule, so the environment suites exercise it.
func TestHostStatusFixtureCarriesVerdictRule(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	var names []string
	require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type = 'trigger' AND tbl_name = 'environments' ORDER BY name").Scan(&names).Error)
	require.Equal(t, []string{"environments_no_reinsert", "environments_revision_monotonic", "environments_verdict_update"}, names)
	require.NoError(t, db.Create(&Environment{ID: "fixture", ApiUrl: "http://fixture.example.com", Status: "update-required"}).Error)
	require.ErrorContains(t, db.Model(&Environment{}).Where("id = ?", "fixture").Update("status", "online").Error, "host.status-verdict")
	require.NoError(t, db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Environment{ID: "fixture", ApiUrl: "http://fixture.example.com", Status: "online"}).Error)
	var status string
	require.NoError(t, db.Raw("SELECT status FROM environments WHERE id = 'fixture'").Scan(&status).Error)
	require.Equal(t, "update-required", status, "a re-insert never changes the row")
}

// Row 4: a database migrated before 004 keeps every status, gains the default revision
// and refuses a bypass after reopening.
func TestHostVerdictRuleForwardMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	before := fstest.MapFS{}
	for _, name := range []string{"001_baseline.sql", "002_candidate_scans.sql", "003_update_recovery.sql"} {
		data, err := resources.FS.ReadFile("migrations/sqlite/" + name)
		require.NoError(t, err)
		before[name] = &fstest.MapFile{Data: data}
	}
	db, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{Source: before})
	require.NoError(t, err)
	for id, status := range map[string]string{"forward-verdict": "update-required", "forward-pending": "pending", "forward-online": "online"} {
		require.NoError(t, db.Exec("INSERT INTO environments (id, created_at, api_url, status) VALUES (?, CURRENT_TIMESTAMP, ?, ?)", id, "http://"+id+".example.com", status).Error)
	}
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.Equal(t, hostRow{"update-required", 0, 0}, readHostRow(t, db.DB, "forward-verdict"))
	require.Equal(t, hostRow{"pending", 0, 0}, readHostRow(t, db.DB, "forward-pending"))
	require.Equal(t, hostRow{"online", 0, 0}, readHostRow(t, db.DB, "forward-online"))
	var triggers []sql.NullString
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'trigger' ORDER BY name").Scan(&triggers).Error)
	require.Len(t, triggers, 3)
	require.ErrorContains(t, db.Exec("UPDATE environments SET status = 'online' WHERE id = 'forward-verdict'").Error, "host.status-verdict")
}

// The generic edit sink accepts only its editable columns, so no spelling of the status
// column (GORM field name, SQLite case variants) can replace a verdict; refused edits
// change nothing and successful edits persist and keep it.
func TestGenericHostUpdateRefusesStatus(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("status", "update-required").Error)
	before, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	stored := func(label string) *Environment {
		t.Helper()
		row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
		require.NoError(t, err)
		require.Equal(t, "update-required", row.Status, label)
		return row
	}
	for _, key := range []string{"status", "Status", "STATUS", "sTaTuS", "last_seen", "is_edge"} {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{key: "pending", "name": "Renamed"}, nil, nil)
		require.Error(t, err, key)
		require.Equal(t, before.Name, stored(key).Name, key)
	}
	edits := []struct {
		label string
		edit  map[string]any
		check func(*Environment) bool
	}{
		{"name", map[string]any{"name": "Example renamed"}, func(e *Environment) bool { return e.Name == "Example renamed" }},
		{"colour set", map[string]any{"color": "#336699"}, func(e *Environment) bool { return e.Color != nil && *e.Color == "#336699" }},
		{"colour null", map[string]any{"color": nil}, func(e *Environment) bool { return e.Color == nil }},
		{"disable", map[string]any{"enabled": false}, func(e *Environment) bool { return !e.Enabled }},
		{"enable", map[string]any{"enabled": true}, func(e *Environment) bool { return e.Enabled }},
	}
	for _, step := range edits {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, step.edit, nil, nil)
		require.NoError(t, err, step.label)
		require.True(t, step.check(stored(step.label)), step.label)
	}
}
