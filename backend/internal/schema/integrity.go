// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0 AND BSD-3-Clause
// Deviations: Empty-model schema generation using the ported retained integrity inventory.

package schema

import (
	"embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

//go:embed retained_integrity.json
var integrityFS embed.FS

type columnRule struct {
	Check   string  `json:"check"`
	Table   string  `json:"table"`
	Column  string  `json:"column"`
	NotNull bool    `json:"notNull"`
	Default *string `json:"default"`
}
type integrityRules struct {
	Columns     []columnRule
	Constraints map[string][]string
	Indexes     []string
}

// ApplyIntegrity is development-only: it refuses data and rebuilds empty model
// tables with the independently retained constraints before producing a baseline.
func ApplyIntegrity(db *gorm.DB) error {
	data, err := integrityFS.ReadFile("retained_integrity.json")
	if err != nil {
		return err
	}
	var rules integrityRules
	if err = json.Unmarshal(data, &rules); err != nil {
		return err
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return err
	}
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		var count int64
		if err = db.Table(table).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("integrity generation requires empty tables")
		}
	}
	if err = db.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		return err
	}
	defer db.Exec("PRAGMA foreign_keys=ON")
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		if err = rebuildEmpty(db, table, rules); err != nil {
			return err
		}
	}
	for _, statement := range rules.Indexes {
		statement = strings.Replace(statement, " INDEX ", " INDEX IF NOT EXISTS ", 1)
		if err = db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

type column struct {
	CID       int
	Name      string
	Type      string
	Notnull   int
	DfltValue *string
	PK        int
}

func rebuildEmpty(db *gorm.DB, table string, rules integrityRules) error {
	var columns []column
	if err := db.Raw("PRAGMA table_info(" + quote(table) + ")").Scan(&columns).Error; err != nil {
		return err
	}
	var original string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&original).Error; err != nil {
		return err
	}
	var indexes []string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name=? AND sql IS NOT NULL", table).Scan(&indexes).Error; err != nil {
		return err
	}
	pk := []string{}
	definitions := []string{}
	for _, c := range columns {
		if c.PK != 0 {
			pk = append(pk, quote(c.Name))
		}
	}
	for _, c := range columns {
		rule := columnRule{}
		for _, candidate := range rules.Columns {
			if candidate.Table == table && candidate.Column == c.Name {
				rule = candidate
				break
			}
		}
		def := quote(c.Name) + " " + c.Type
		if len(pk) == 1 && c.PK != 0 && strings.EqualFold(c.Type, "integer") {
			def += " PRIMARY KEY"
			if strings.Contains(original, "AUTOINCREMENT") {
				def += " AUTOINCREMENT"
			}
		}
		if c.Notnull != 0 || rule.NotNull {
			def += " NOT NULL"
		}
		value := c.DfltValue
		if rule.Default != nil {
			value = rule.Default
		}
		if value != nil {
			def += " DEFAULT " + *value
		}
		if rule.Check != "" {
			def += " " + rule.Check
		}
		definitions = append(definitions, def)
	}
	if len(pk) > 0 && !(len(pk) == 1 && strings.Contains(strings.Join(definitions, ","), " PRIMARY KEY")) {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(pk, ",")+")")
	}
	definitions = append(definitions, rules.Constraints[table]...)
	// JSON fields retain SQL NULL separately from valid JSON null, arrays and objects.
	for _, model := range Models() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return err
		}
		if stmt.Table != table {
			continue
		}
		for _, field := range stmt.Schema.Fields {
			name := field.FieldType.Name()
			if field.Serializer != nil || name == "JSON" || name == "StringSlice" || name == "OutputLines" || name == "EnvironmentUpdateResults" {
				definitions = append(definitions, "CHECK ("+quote(field.DBName)+" IS NULL OR json_valid("+quote(field.DBName)+"))")
			}
		}
	}
	if err := db.Exec("DROP TABLE " + quote(table)).Error; err != nil {
		return err
	}
	if err := db.Exec("CREATE TABLE " + quote(table) + " (" + strings.Join(definitions, ",") + ")").Error; err != nil {
		return err
	}
	for _, index := range indexes {
		if err := db.Exec(index).Error; err != nil {
			return err
		}
	}
	return nil
}
func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
