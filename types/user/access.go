// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user

type AccessCode string

const (
	AccessNotFound      AccessCode = "access.not-found"
	AccessAdminRequired AccessCode = "access.admin-required"
	AccessFullRequired  AccessCode = "access.full-required"
	AccessDisabled      AccessCode = "access.disabled"
	AccessNotRegistered AccessCode = "access.not-registered"
	AccessUnavailable   AccessCode = "access.unavailable"
)

type AccessProblem struct {
	Code   AccessCode `json:"code" enum:"access.not-found,access.admin-required,access.full-required,access.disabled,access.not-registered,access.unavailable"`
	Status int        `json:"status"`
	Detail string     `json:"detail,omitempty"`
}

func (p *AccessProblem) Error() string             { return string(p.Code) }
func (p *AccessProblem) GetStatus() int            { return p.Status }
func (p *AccessProblem) ContentType(string) string { return "application/problem+json" }
