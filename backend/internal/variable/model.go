// Source: getarcaneapp/arcane/backend/internal/variable/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package variable

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
)

// GlobalVariable is a manager-level key/value variable materialized into each
// environment's .env.global file. Value holds ciphertext when IsSecret is true.
type GlobalVariable struct {
	database.BaseModel

	Key      string `json:"key" gorm:"column:key" sortable:"true"`
	Value    string `json:"-" gorm:"column:value"`
	IsSecret bool   `json:"isSecret" gorm:"column:is_secret"`
	// No gorm default tag: a default:true tag would make GORM omit false on
	// insert and the column default would silently win.
	AllEnvironments bool                      `json:"allEnvironments" gorm:"column:all_environments"`
	Environments    []environment.Environment `json:"-" gorm:"many2many:global_variable_environments;joinForeignKey:GlobalVariableID;joinReferences:EnvironmentID"`
}

func (GlobalVariable) TableName() string { return "global_variables" }
