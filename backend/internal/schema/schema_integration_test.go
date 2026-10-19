// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Bidirectional SQLite schema proof and independent retained-integrity assertions.

package schema

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/retained-integrity.json
var retainedIntegrityReference []byte

func freshDatabase(t *testing.T) string {
	t.Helper()
	return sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
}
func canonical(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.NewReplacer(`"`, "", "`", "", "(", " ", ")", " ", ",", " ").Replace(s)), " "))
}
func sqlRows(t *testing.T, db *sql.DB, query string, args ...any) [][]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer rows.Close()
	names, err := rows.Columns()
	require.NoError(t, err)
	var result [][]string
	for rows.Next() {
		values := make([]sql.NullString, len(names))
		targets := make([]any, len(names))
		for i := range values {
			targets[i] = &values[i]
		}
		require.NoError(t, rows.Scan(targets...))
		row := make([]string, len(names))
		for i, v := range values {
			if v.Valid {
				row[i] = v.String
			} else {
				row[i] = "<NULL>"
			}
		}
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	return result
}
func catalog(t *testing.T, db *sql.DB) []string {
	t.Helper()
	objects := sqlRows(t, db, "SELECT type,name,tbl_name,sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version' AND tbl_name <> 'goose_db_version' ORDER BY type,name")
	var result []string
	for _, o := range objects {
		// SQLite ALTER inserts comma whitespace; it is not a schema property.
        o[3] = strings.ReplaceAll(o[3], ", ", ",")
        result = append(result, strings.Join(o, ":"))
		if o[0] != "table" {
			continue
		}
		for _, pragma := range []string{"table_info", "foreign_key_list", "index_list"} {
			for _, row := range sqlRows(t, db, "PRAGMA "+pragma+"("+quote(o[1])+")") {
				// index_list sequence is creation order, not a schema property.
				if pragma == "index_list" {
					row[0] = ""
					for _, x := range sqlRows(t, db, "PRAGMA index_xinfo("+quote(row[1])+")") {
						result = append(result, "index_xinfo:"+row[1]+":"+strings.Join(x, ":"))
					}
				}
				result = append(result, pragma+":"+o[1]+":"+strings.Join(row, ":"))
			}
		}
	}
	sort.Strings(result)
	return result
}
func TestFreshMigrationMatchesRetainedModels(t *testing.T) {
	migrated, err := database.Initialize(t.Context(), freshDatabase(t), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, migrated.Close()) })
	modelPath, err := sqlitestore.Path(freshDatabase(t))
	require.NoError(t, err)
	pool, err := sqlitestore.Open(t.Context(), modelPath, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	model, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{IgnoreRelationshipsWhenMigrating: true})
	require.NoError(t, err)
	require.NoError(t, model.AutoMigrate(Models()...))
	require.NoError(t, ApplyIntegrity(model))
	require.NoError(t, model.Exec("CREATE TABLE docklet_database_metadata (identity TEXT PRIMARY KEY NOT NULL CHECK (identity = 'docklet-next:sqlite:1'))").Error)
	actual, err := migrated.SQLDB()
	require.NoError(t, err)
	require.Equal(t, catalog(t, pool), catalog(t, actual))
	require.Empty(t, sqlRows(t, actual, "PRAGMA foreign_key_check"))
	require.Equal(t, [][]string{{"ok"}}, sqlRows(t, actual, "PRAGMA integrity_check"))
	var expected [][]string
	require.NoError(t, json.Unmarshal(retainedIntegrityReference, &expected))
	tableSQL := map[string]string{}
	columns := map[string][]string{}
	var allIndexes []string
	for _, row := range sqlRows(t, actual, "SELECT name,sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'") {
		tableSQL[row[0]] = canonical(row[1])
		for _, c := range sqlRows(t, actual, "PRAGMA table_info("+quote(row[0])+")") {
			columns[row[0]+"."+c[1]] = c
		}
	}
	for _, row := range sqlRows(t, actual, "SELECT sql FROM sqlite_master WHERE type='index' AND sql IS NOT NULL") {
		allIndexes = append(allIndexes, canonical(row[0]))
	}
	for table := range tableSQL {
		for _, idx := range sqlRows(t, actual, "PRAGMA index_list("+quote(table)+")") {
			var names []string
			for _, c := range sqlRows(t, actual, "PRAGMA index_xinfo("+quote(idx[1])+")") {
				if c[5] == "1" {
					names = append(names, c[2])
				}
			}
			allIndexes = append(allIndexes, canonical("ON "+table+" ("+strings.Join(names, ",")+")"))
		}
		for _, c := range sqlRows(t, actual, "PRAGMA table_info("+quote(table)+")") {
			if c[5] != "0" && strings.EqualFold(c[2], "integer") {
				allIndexes = append(allIndexes, canonical("ON "+table+" ("+c[1]+")"))
			}
		}
	}
	for _, row := range expected {
		kind, err := integrityReferenceKind(row)
		require.NoError(t, err)
		switch kind {
		case "notnull":
			c := columns[row[1]+"."+row[2]]
			require.NotEmpty(t, c, row)
			require.True(t, c[3] == "1" || (c[5] != "0" && strings.EqualFold(c[2], "integer")), fmt.Sprint(row))
		case "default":
			if row[3] == "INTEGER PRIMARY KEY AUTOINCREMENT" {
				require.Contains(t, tableSQL[row[1]], canonical(row[3]))
				continue
			}
			c := columns[row[1]+"."+row[2]]
			require.NotEmpty(t, c, row)
			require.Equal(t, canonical(row[3]), canonical(c[4]), fmt.Sprint(row))
		case "constraint":
			if strings.HasPrefix(row[2], "PRIMARY KEY (") {
				names := strings.Fields(strings.TrimPrefix(canonical(row[2]), "primary key "))
				actualPK := []string{}
				for _, c := range sqlRows(t, actual, "PRAGMA table_info("+quote(row[1])+")") {
					if c[5] != "0" {
						actualPK = append(actualPK, c[1])
					}
				}
				require.Equal(t, names, actualPK, fmt.Sprint(row))
			} else {
				require.Contains(t, tableSQL[row[1]], canonical(row[2]), fmt.Sprint(row))
			}
		case "index":
			found := false
			for _, index := range allIndexes {
				found = found || strings.Contains(index, canonical(row[2]))
			}
			require.True(t, found, fmt.Sprint(row))
		default:
			t.Fatalf("unhandled integrity reference: %v", row)
		}
	}
	for table := range tableSQL {
		if table == "docklet_database_metadata" {
			continue
		}
		require.Equal(t, [][]string{{"0"}}, sqlRows(t, actual, "SELECT count(*) FROM "+quote(table)))
	}
}

func integrityReferenceKind(row []string) (string, error) {
	if len(row) < 3 {
		return "", errors.New("incomplete integrity reference")
	}
	switch row[0] {
	case "notnull", "constraint", "index":
		if len(row) == 3 {
			return row[0], nil
		}
	case "default":
		if len(row) == 4 {
			return row[0], nil
		}
	default:
		return "", fmt.Errorf("unknown integrity reference kind: %s", row[0])
	}
	return "", fmt.Errorf("invalid integrity reference arity: %s", row[0])
}

func TestIntegrityReferenceRejectsUnknownAndMalformedFacts(t *testing.T) {
	for _, row := range [][]string{nil, {"constraint"}, {"check", "runtime_receipts", "CHECK (0)"},
		{"future-kind", "runtime_receipts", "CHECK (0)"}, {"default", "runtime_receipts", "kind"},
		{"index", "runtime_receipts", "ON runtime_receipts(run_id)", "ignored"}} {
		_, err := integrityReferenceKind(row)
		require.Error(t, err, "every expected fact must be interpreted: %v", row)
	}
	var rows [][]string
	require.NoError(t, json.Unmarshal(retainedIntegrityReference, &rows))
	// Keep dispatch evidence independently specified even when SQL and overlay agree.
	require.Contains(t, rows, []string{"constraint", "runtime_receipts", "CHECK ((dispatch_state IS NOT NULL AND dispatch_state IN ('dispatching','accepted')) = (dispatched_at IS NOT NULL))"})
	for _, row := range rows {
		_, err := integrityReferenceKind(row)
		require.NoError(t, err, "%v", row)
	}
}

func runtimeReceiptFixture() map[string]any {
	return map[string]any{
		"run_id": "runtime-run", "kind": "receipt", "store_generation": "own-generation",
		"manager_id": "manager", "registration_id": "registered-target", "environment_id": "0",
		"executor_generation": "bound-executor-generation", "request_digest": "canonical-digest",
		"caller_id": "actor", "caller_name": "Original actor", "operation_id": "start-container",
		"target_kind": "container", "target_id": "full-container-id", "target_name": "Example container",
		"parent_project_id": "project", "gate_scope": "container:full-container-id", "gate_key": "start:semantic-digest",
		"admitted_at": time.Unix(1760000000, 123456000).UTC(), "dispatch_state": "pending",
		"executor_state": "pending", "cancel_requested": false, "release_acknowledged": false,
		"final_write_at": time.Unix(1760000000, 123456000).UTC(),
	}
}

func runtimeSchemaDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Initialize(t.Context(), freshDatabase(t), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestRuntimeSchemaRejectsInvalidEvidence(t *testing.T) {
	db := runtimeSchemaDB(t)
	stamp := time.Unix(1760000001, 654321000).UTC()
	for _, test := range []struct {
		name  string
		patch map[string]any
	}{
		{"empty_run", map[string]any{"run_id": ""}},
		{"unknown_kind", map[string]any{"kind": "other"}},
		{"empty_store_generation", map[string]any{"store_generation": ""}},
		{"missing_executor_generation", map[string]any{"executor_generation": nil}},
		{"empty_manager", map[string]any{"manager_id": ""}},
		{"empty_registration", map[string]any{"registration_id": ""}},
		{"empty_environment", map[string]any{"environment_id": ""}},
		{"empty_digest", map[string]any{"request_digest": ""}},
		{"missing_caller", map[string]any{"caller_id": nil}},
		{"empty_caller", map[string]any{"caller_id": ""}},
		{"missing_operation", map[string]any{"operation_id": nil}},
		{"unknown_operation", map[string]any{"operation_id": "kill-container"}},
		{"wrong_target_kind", map[string]any{"target_kind": "stack"}},
		{"missing_target", map[string]any{"target_id": nil}},
		{"empty_scope", map[string]any{"gate_scope": ""}},
		{"missing_key", map[string]any{"gate_key": nil}},
		{"missing_admission", map[string]any{"admitted_at": nil}},
		{"empty_parent", map[string]any{"parent_project_id": ""}},
		{"unknown_dispatch", map[string]any{"dispatch_state": "sent"}},
		{"missing_dispatch", map[string]any{"dispatch_state": nil}},
		{"dispatching_without_time", map[string]any{"dispatch_state": "dispatching"}},
		{"accepted_without_time", map[string]any{"dispatch_state": "accepted", "executor_state": "accepted"}},
		{"time_without_dispatch", map[string]any{"dispatched_at": stamp}},
		{"unknown_executor_state", map[string]any{"executor_state": "done"}},
		{"missing_executor_state", map[string]any{"executor_state": nil}},
		{"unknown_without_reason", map[string]any{"executor_state": "unknown"}},
		{"reason_without_unknown", map[string]any{"unknown_reason": "transport"}},
		{"empty_unknown_reason", map[string]any{"executor_state": "unknown", "unknown_reason": ""}},
		{"terminal_without_executor_time", map[string]any{"executor_state": "success", "settled_at": stamp}},
		{"executor_time_without_terminal", map[string]any{"executor_settled_at": stamp, "settled_at": stamp}},
		{"error_before_settlement", map[string]any{"executor_error": "unsettled"}},
		{"result_before_settlement", map[string]any{"executor_result_code": "runtime.failed"}},
		{"cancel_without_time", map[string]any{"cancel_requested": true}},
		{"cancel_time_without_intent", map[string]any{"cancel_requested_at": stamp}},
		{"invalid_cancel_boolean", map[string]any{"cancel_requested": 2, "cancel_requested_at": stamp}},
		{"disposition_without_audit", map[string]any{"hub_disposition": "released", "settled_at": stamp}},
		{"audit_without_disposition", map[string]any{"hub_disposition_by": "admin"}},
		{"unknown_disposition", map[string]any{"hub_disposition": "success", "hub_disposition_by": "admin", "hub_disposition_reason": "reason", "hub_disposition_at": stamp, "settled_at": stamp}},
		{"acknowledgment_without_release", map[string]any{"release_acknowledged": true}},
		{"release_without_acknowledgment", map[string]any{"hub_disposition": "released", "hub_disposition_by": "admin", "hub_disposition_reason": "reason", "hub_disposition_at": stamp, "settled_at": stamp}},
		{"premature_settlement", map[string]any{"settled_at": stamp}},
		{"missing_final_write", map[string]any{"final_write_at": nil}},
		{"tombstone_with_admission", map[string]any{"kind": "not_received", "settled_at": stamp}},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := runtimeReceiptFixture()
			maps.Copy(row, test.patch)
			err := db.Table("runtime_receipts").Create(maps.Clone(row)).Error
			// Keep successful mutants isolated so each assertion tests its own constraint.
			if err == nil {
				require.NoError(t, db.Exec("DELETE FROM runtime_receipts").Error)
			}
			require.ErrorContains(t, err, "constraint failed", "invalid compact evidence must be rejected")
		})
	}
}

func TestRuntimeSchemaDispatchEvidenceSurvivesReopen(t *testing.T) {
	url := freshDatabase(t)
	db, err := database.Initialize(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	row := runtimeReceiptFixture()
	row["environment_id"] = "registered-target"
	require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error)
	read := func(id string) activity.RuntimeReceipt {
		t.Helper()
		var receipt activity.RuntimeReceipt
		require.NoError(t, db.First(&receipt, "run_id = ?", id).Error)
		return receipt
	}
	before := read("runtime-run")
	require.Equal(t, "pending", *before.DispatchState)
	require.Nil(t, before.DispatchedAt)
	dispatched := row["admitted_at"].(time.Time).Add(time.Second)
	accepted := dispatched.Add(time.Minute)
	// DispatchState records the hub send marker; ExecutorState records acceptance/work.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("UPDATE runtime_receipts SET dispatch_state='dispatching',dispatched_at=?,final_write_at=? WHERE run_id='runtime-run'", dispatched, dispatched).Error
	}))
	sending := read("runtime-run")
	require.Equal(t, "dispatching", *sending.DispatchState)
	require.Equal(t, dispatched, *sending.DispatchedAt)
	require.Equal(t, "pending", *sending.ExecutorState)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("UPDATE runtime_receipts SET dispatch_state='accepted',executor_state='accepted',final_write_at=? WHERE run_id='runtime-run'", accepted).Error
	}))
	acknowledged := read("runtime-run")
	want := sending
	want.DispatchState, want.ExecutorState, want.FinalWriteAt = new("accepted"), new("accepted"), accepted
	require.Equal(t, want, acknowledged)
	require.Equal(t, dispatched, *acknowledged.DispatchedAt)

	// Executor-local acceptance has no hub send to invent, including local execution.
	for _, state := range []string{"accepted", "executing"} {
		executor := runtimeReceiptFixture()
		executor["run_id"], executor["gate_scope"] = "executor-"+state, "container:executor-"+state
		executor["executor_generation"], executor["executor_state"] = "own-generation", state
		if state == "executing" {
			executor["executor_started_at"] = accepted.Add(time.Hour)
		}
		require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(executor)).Error)
	}
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	require.Equal(t, acknowledged, read("runtime-run"))
	pool, err := db.SQLDB()
	require.NoError(t, err)
	require.Equal(t, [][]string{{fmt.Sprint(dispatched.UnixMicro())}}, sqlRows(t, pool, "SELECT CAST(dispatched_at AS INTEGER) FROM runtime_receipts WHERE run_id='runtime-run'"))
	for _, state := range []string{"accepted", "executing"} {
		executor := read("executor-" + state)
		require.Equal(t, "0", executor.EnvironmentID)
		require.Equal(t, executor.StoreGeneration, executor.ExecutorGeneration)
		require.Equal(t, "pending", *executor.DispatchState)
		require.Nil(t, executor.DispatchedAt)
		require.Equal(t, state, *executor.ExecutorState)
		if state == "executing" {
			require.Equal(t, accepted.Add(time.Hour), *executor.ExecutorStartedAt)
		}
	}
	// An independent hub release and later executor result retain the original send.
	released := accepted.Add(time.Hour)
	late := released.Add(time.Hour)
	require.NoError(t, db.Exec("UPDATE runtime_receipts SET executor_state='unknown',unknown_reason='transport',hub_disposition='released',release_acknowledged=1,hub_disposition_by='admin',hub_disposition_reason='Executor stopped and state inspected',hub_disposition_at=?,settled_at=?,final_write_at=? WHERE run_id='runtime-run'", released, released, released).Error)
	disposed := read("runtime-run")
	require.Equal(t, dispatched, *disposed.DispatchedAt)
	require.Equal(t, "accepted", *disposed.DispatchState)
	require.Equal(t, "unknown", *disposed.ExecutorState)
	require.NoError(t, db.Exec("UPDATE runtime_receipts SET executor_state='success',unknown_reason=NULL,executor_result_code='runtime.succeeded',executor_settled_at=?,final_write_at=? WHERE run_id='runtime-run'", late, late).Error)
	want = disposed
	want.ExecutorState, want.UnknownReason, want.ExecutorResultCode = new("success"), nil, new("runtime.succeeded")
	want.ExecutorSettledAt, want.FinalWriteAt = &late, late
	require.Equal(t, want, read("runtime-run"))
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	require.Equal(t, want, read("runtime-run"))
}

func TestRuntimeSchemaIdentityRetentionAndLateEvidence(t *testing.T) {
	db := runtimeSchemaDB(t)
	row := runtimeReceiptFixture()
	require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error)
	// The run UUID is unique across the whole database, including other environments.
	other := maps.Clone(row)
	other["environment_id"] = "other-registration"
	require.Error(t, db.Table("runtime_receipts").Create(maps.Clone(other)).Error)
	other["run_id"] = "other-run"
	require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(other)).Error)
	other["run_id"], other["environment_id"] = "same-scope", "0"
	require.Error(t, db.Table("runtime_receipts").Create(maps.Clone(other)).Error)
	// Siblings share a parent but have distinct scopes.
	other["gate_scope"], other["target_id"] = "container:sibling", "sibling"
	require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(other)).Error)
	stamp := row["admitted_at"].(time.Time).Add(time.Hour)
	key := map[string]any{"environment_id": "0", "idempotency_key": "opaque-key", "run_id": "runtime-run", "caller_id": "joining-actor", "caller_name": "Joining actor", "request_digest": "canonical-digest", "first_seen_at": stamp}
	require.NoError(t, db.Table("runtime_key_bindings").Create(maps.Clone(key)).Error)
	duplicate := maps.Clone(key)
	duplicate["caller_id"], duplicate["request_digest"] = "different-actor", "different-digest"
	require.Error(t, db.Table("runtime_key_bindings").Create(maps.Clone(duplicate)).Error)
	duplicate["run_id"] = "same-scope"
	require.Error(t, db.Table("runtime_key_bindings").Create(maps.Clone(duplicate)).Error)
	duplicate["environment_id"], duplicate["run_id"] = "other-registration", "other-run"
	require.NoError(t, db.Table("runtime_key_bindings").Create(maps.Clone(duplicate)).Error)
	wrong := maps.Clone(key)
	wrong["idempotency_key"], wrong["environment_id"] = "wrong-registration", "missing-registration"
	require.Error(t, db.Table("runtime_key_bindings").Create(maps.Clone(wrong)).Error)
	wrong["environment_id"], wrong["run_id"] = "0", "absent-run"
	require.Error(t, db.Table("runtime_key_bindings").Create(maps.Clone(wrong)).Error)
	for _, column := range []string{"environment_id", "idempotency_key", "run_id", "caller_id", "request_digest", "first_seen_at"} {
		wrong = maps.Clone(key)
		wrong["idempotency_key"], wrong[column] = "invalid-key", nil
		require.Error(t, db.Table("runtime_key_bindings").Create(maps.Clone(wrong)).Error, column)
	}
	// Existing activity, caller and environment deletion cannot erase compact evidence.
	for _, sql := range []string{
		"INSERT INTO users(id,username,password_hash) VALUES('actor','example-actor','fixture-hash')",
		"INSERT INTO environments(id,api_url) VALUES('registered-target','https://executor.example')",
		"INSERT INTO activities(id,created_at,environment_id,type,status,started_at) VALUES('runtime-run',1760000000000000,'0','container_start','success',1760000000000000)",
		"DELETE FROM activities WHERE id='runtime-run'", "DELETE FROM users WHERE id='actor'", "DELETE FROM environments WHERE id='registered-target'",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	var receipt activity.RuntimeReceipt
	require.NoError(t, db.First(&receipt, "run_id = ?", "runtime-run").Error)
	require.Equal(t, "actor", *receipt.CallerID)
	require.Equal(t, "own-generation", receipt.StoreGeneration)
	require.Equal(t, "bound-executor-generation", receipt.ExecutorGeneration)
	require.Equal(t, row["admitted_at"], *receipt.AdmittedAt)
	// Unknown stays held until a separately audited hub disposition settles it.
	require.NoError(t, db.Exec("UPDATE runtime_receipts SET executor_state='unknown',unknown_reason='transport' WHERE run_id='runtime-run'").Error)
	require.NoError(t, db.Exec("UPDATE runtime_receipts SET hub_disposition='released',release_acknowledged=1,hub_disposition_by='admin',hub_disposition_reason='Executor stopped and state inspected',hub_disposition_at=?,settled_at=?,final_write_at=? WHERE run_id='runtime-run'", stamp, stamp, stamp).Error)
	late := stamp.Add(time.Minute)
	require.NoError(t, db.Exec("UPDATE runtime_receipts SET executor_state='success',unknown_reason=NULL,executor_result_code='runtime.succeeded',executor_settled_at=?,final_write_at=? WHERE run_id='runtime-run'", late, late).Error)
	var released activity.RuntimeReceipt
	require.NoError(t, db.First(&released, "run_id = ?", "runtime-run").Error)
	require.Equal(t, "released", *released.HubDisposition)
	require.Equal(t, stamp, *released.HubDispositionAt)
	require.Equal(t, stamp, *released.SettledAt)
	require.Equal(t, "success", *released.ExecutorState)
	require.Equal(t, late, *released.ExecutorSettledAt)
	var timeType string
	require.NoError(t, db.Raw("SELECT typeof(final_write_at) FROM runtime_receipts WHERE run_id='runtime-run'").Scan(&timeType).Error)
	require.Equal(t, "integer", timeType)
	// Restrict deletion until the complete compact set is removed atomically.
	require.Error(t, db.Exec("DELETE FROM runtime_receipts WHERE run_id='runtime-run'").Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM runtime_key_bindings WHERE run_id='runtime-run'").Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM runtime_receipts WHERE run_id='runtime-run'").Error
	}))
	pool, err := db.SQLDB()
	require.NoError(t, err)
	require.Empty(t, sqlRows(t, pool, "PRAGMA foreign_key_check"))
	require.Equal(t, [][]string{{"ok"}}, sqlRows(t, pool, "PRAGMA integrity_check"))
}

func TestRuntimeSchemaTombstoneAndGeneration(t *testing.T) {
	db := runtimeSchemaDB(t)
	stamp := time.Unix(1760000000, 123456000).UTC()
	tombstone := map[string]any{"run_id": "negative-run", "kind": "not_received", "store_generation": "own-generation", "manager_id": "manager", "registration_id": "registered-target", "environment_id": "0", "executor_generation": "own-generation", "request_digest": "canonical-digest", "cancel_requested": false, "release_acknowledged": false, "settled_at": stamp, "final_write_at": stamp}
	require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(tombstone)).Error)
	for column, value := range map[string]any{"caller_id": "actor", "caller_name": "name", "operation_id": "start-container", "target_id": "target", "gate_scope": "container:target", "admitted_at": stamp, "dispatch_state": "accepted", "dispatched_at": stamp, "executor_state": "pending", "cancel_requested": true, "hub_disposition_by": "admin", "settled_at": nil} {
		row := maps.Clone(tombstone)
		row["run_id"], row[column] = "invalid-negative", value
		err := db.Table("runtime_receipts").Create(maps.Clone(row)).Error
		if err == nil {
			require.NoError(t, db.Exec("DELETE FROM runtime_receipts WHERE run_id='invalid-negative'").Error)
		}
		require.ErrorContains(t, err, "constraint failed", column)
	}
	admission := runtimeReceiptFixture()
	admission["run_id"] = "negative-run"
	require.Error(t, db.Table("runtime_receipts").Create(maps.Clone(admission)).Error)
	for _, id := range []int{0, 2, -1} {
		require.Error(t, db.Create(&activity.RuntimeStoreGeneration{ID: id, Generation: "generation", CreatedAt: stamp}).Error)
	}
	require.Error(t, db.Create(&activity.RuntimeStoreGeneration{ID: 1, Generation: "", CreatedAt: stamp}).Error)
	require.NoError(t, db.Create(&activity.RuntimeStoreGeneration{ID: 1, Generation: "generation", CreatedAt: stamp}).Error)
	require.Error(t, db.Create(&activity.RuntimeStoreGeneration{ID: 1, Generation: "another", CreatedAt: stamp}).Error)
}

func TestRuntimeSchemaAdmitsRequiredStates(t *testing.T) {
	db := runtimeSchemaDB(t)
	stamp := time.Unix(1760000000, 654321000).UTC()
	for _, operation := range []string{"deploy-project", "down-project", "restart-project", "redeploy-project", "start-container", "stop-container", "restart-container"} {
		row := runtimeReceiptFixture()
		row["run_id"], row["operation_id"], row["gate_scope"] = operation, operation, "scope:"+operation
		if strings.HasSuffix(operation, "project") {
			row["target_kind"], row["parent_project_id"] = "stack", nil
		}
		require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error, operation)
	}
	for _, test := range []struct {
		name  string
		patch map[string]any
	}{
		{"dispatching", map[string]any{"dispatch_state": "dispatching", "dispatched_at": stamp}},
		{"hub_accepted", map[string]any{"dispatch_state": "accepted", "dispatched_at": stamp, "executor_state": "accepted"}},
		{"hub_executing", map[string]any{"dispatch_state": "accepted", "dispatched_at": stamp, "executor_state": "executing", "executor_started_at": stamp}},
		{"executor_accepted", map[string]any{"executor_state": "accepted"}},
		{"executor_executing", map[string]any{"executor_state": "executing", "executor_started_at": stamp}},
		{"cancelling", map[string]any{"cancel_requested": true, "cancel_requested_at": stamp}},
		{"unknown", map[string]any{"executor_state": "unknown", "unknown_reason": "transport"}},
		{"success", map[string]any{"executor_state": "success", "executor_settled_at": stamp, "settled_at": stamp}},
		{"failed", map[string]any{"executor_state": "failed", "executor_error": "interrupted", "executor_result_code": "runtime.interrupted", "executor_settled_at": stamp, "settled_at": stamp}},
		{"cancelled", map[string]any{"executor_state": "cancelled", "executor_settled_at": stamp, "settled_at": stamp}},
	} {
		row := runtimeReceiptFixture()
		row["run_id"], row["gate_scope"] = test.name, "scope:"+test.name
		maps.Copy(row, test.patch)
		require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error, test.name)
	}
	for _, disposition := range []string{"cancelled", "interrupted", "released", "refused", "not_received"} {
		row := runtimeReceiptFixture()
		row["run_id"], row["gate_scope"] = "hub-"+disposition, "scope:hub-"+disposition
		row["release_acknowledged"] = disposition == "released"
		row["hub_disposition"], row["hub_disposition_by"], row["hub_disposition_reason"], row["hub_disposition_at"], row["settled_at"] = disposition, "hub-writer", "audited reason", stamp, stamp
		require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error, disposition)
		if disposition == "released" {
			// Settled evidence does not continue to hold an equal active scope.
			row["run_id"], row["release_acknowledged"] = "replacement-run", false
			for _, column := range []string{"hub_disposition", "hub_disposition_by", "hub_disposition_reason", "hub_disposition_at", "settled_at"} {
				delete(row, column)
			}
			require.NoError(t, db.Table("runtime_receipts").Create(maps.Clone(row)).Error)
		}
	}
}
