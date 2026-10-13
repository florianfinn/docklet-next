// Source: getarcaneapp/arcane/types/user/actor.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package user

// Actor identifies a user performing domain work without exposing persistence data.
type Actor struct {
	ID          string      `json:"id"`
	Username    string      `json:"username"`
	DisplayName *string     `json:"displayName,omitempty"`
	Preferences Preferences `json:"preferences"`
}

// ActorProvider supplies an actor from an authenticated user model.
type ActorProvider interface{ Actor() *Actor }

// CurrentUserContextKey holds the authenticated user or actor in a request context.
type CurrentUserContextKey struct{}

// SystemUser identifies work initiated by Arcane itself.
var SystemUser = Actor{Username: "System"}
