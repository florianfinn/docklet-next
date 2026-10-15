// Source: getarcaneapp/arcane/backend/internal/event/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; extracted actor normalization.

package event

import "strings"

func normalizeEventActor(userID, username *string) (*string, *string) {
	normalizedUserID := normalizeOptionalStringPtr(userID)
	normalizedUsername := normalizeOptionalStringPtr(username)

	if normalizedUsername == nil && normalizedUserID != nil {
		normalizedUsername = new(*normalizedUserID)
	}
	if normalizedUserID == nil && normalizedUsername != nil {
		normalizedUserID = new(*normalizedUsername)
	}
	if normalizedUserID == nil && normalizedUsername == nil {
		normalizedUserID = new("system")
		normalizedUsername = new("System")
	}

	return normalizedUserID, normalizedUsername
}

func normalizeOptionalStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
