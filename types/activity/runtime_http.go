// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Shared admission, conflict and legacy failure response vocabulary.

package activity

type RuntimeID struct {
	ActivityID string `json:"activityId"`
}
type RuntimeHTTPError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}
type RuntimeFailureResponse struct {
	Success bool             `json:"success"`
	Error   RuntimeHTTPError `json:"error"`
	Data    RuntimeID        `json:"data"`
}
type RuntimeConflict struct {
	Code        Code   `json:"code"`
	ActiveRunID string `json:"activeRunId,omitempty"`
}
type RuntimeConflictResponse struct {
	Success bool            `json:"success"`
	Data    RuntimeConflict `json:"data"`
}
type RuntimeStreamFailure struct {
	Error      string `json:"error"`
	Code       Code   `json:"code"`
	ActivityID string `json:"activityId"`
}
