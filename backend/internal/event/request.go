// Source: getarcaneapp/arcane/backend/internal/event/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; coded event request.

package event

import "github.com/florianfinn/docklet-next/backend/internal/database"

type CreateEventRequest struct {
	// Internal identity for replay-safe daemon delivery; never accepted from API payloads.
	deduplicationKey string
	Type             EventType     `json:"type"`
	MessageCode      string        `json:"messageCode,omitempty"`
	Severity         EventSeverity `json:"severity,omitempty"`
	Title            string        `json:"title"`
	Description      string        `json:"description,omitempty"`
	ResourceType     *string       `json:"resourceType,omitempty"`
	ResourceID       *string       `json:"resourceId,omitempty"`
	ResourceName     *string       `json:"resourceName,omitempty"`
	UserID           *string       `json:"userId,omitempty"`
	Username         *string       `json:"username,omitempty"`
	EnvironmentID    *string       `json:"environmentId,omitempty"`
	Metadata         database.JSON `json:"metadata,omitempty"`
}
