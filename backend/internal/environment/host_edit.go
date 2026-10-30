// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host validation and credential generation fencing.

package environment

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

var hostColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func hostProblem(code string, status int) error {
	return &environment.HostProblem{Code: code, Status: status}
}

// Call only inside the same write transaction as registration or rename. SQLite
// BEGIN IMMEDIATE serializes competing writers before this uniqueness read.
func validateHostName(tx *gorm.DB, id, name string) error {
	if strings.TrimSpace(name) != name || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 40 {
		return hostProblem("host.name-invalid", 400)
	}
	var rows []Environment
	if err := tx.Select("id", "name").Where("id <> ?", id).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if strings.EqualFold(row.Name, name) {
			return hostProblem("host.name-taken", 409)
		}
	}
	return nil
}

func validateHostColor(value any) error {
	if value == nil {
		return nil
	}
	color, ok := value.(string)
	if !ok || !hostColorPattern.MatchString(color) {
		return errors.New("color must be a lowercase #rrggbb value or null")
	}
	return nil
}

// SQL increments fence all simultaneous writers without a read/modify/write gap.
func fenceCredentialUpdate(updates map[string]any) {
	for _, field := range []string{"api_url", "api_key_id", "access_token"} {
		if _, ok := updates[field]; ok {
			updates["credential_revision"] = gorm.Expr("credential_revision + 1")
			return
		}
	}
}
