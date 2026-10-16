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
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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
		switch row[0] {
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
		}
	}
	for table := range tableSQL {
		if table == "docklet_database_metadata" {
			continue
		}
		require.Equal(t, [][]string{{"0"}}, sqlRows(t, actual, "SELECT count(*) FROM "+quote(table)))
	}
}
