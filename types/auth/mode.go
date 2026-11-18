// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original sign-in mode vocabulary; M15 simple-user rights and identity contracts; M29 stored sign-in policy and first-run identity claim.

package auth

type SignInMode string

const (
	SignInModeNone  SignInMode = "none"
	SignInModeLocal SignInMode = "local"
	SignInModeProxy SignInMode = "proxy"
)

type SignInConfiguration struct {
	LogoutURL      *string    `json:"logoutUrl,omitempty" readOnly:"true"`
	Mode           SignInMode `json:"mode" enum:"none,local,proxy" readOnly:"true" doc:"Active sign-in mode; while setup is pending the forced mode or local as the default choice"`
	SetupPending   bool       `json:"setupPending" readOnly:"true" doc:"True until first-run setup has created the first administrator"`
	Forced         bool       `json:"forced" readOnly:"true" doc:"True when AUTH_MODE fixes the mode; setup and settings cannot change it"`
	ProxyHeader    *string    `json:"proxyHeader,omitempty" readOnly:"true" doc:"Proxy identity header; returned by GET /auth/sign-in and while setup is pending"`
	TrustedProxies *string    `json:"trustedProxies,omitempty" readOnly:"true" doc:"Trusted proxy addresses; returned by GET /auth/sign-in and while setup is pending"`
}

// Problem codes of the first-run claim and the sign-in policy writes.
const (
	CodeSetupPending     = "setup.pending"
	CodeSetupNotPending  = "setup.not-pending"
	CodeSetupCodeInvalid = "setup.code-invalid"
	CodeSetupRateLimited = "setup.rate-limited"
	CodeModeForced       = "auth.mode-forced"
	CodeProxyUnverified  = "auth.proxy-unverified"
	CodePasswordRequired = "auth.password-required"
	CodeUsernameTaken    = "user.username-taken"
)

// SignInPolicyInput is the proxy part shared by the claim and the settings write.
type SignInPolicyInput struct {
	Mode           SignInMode `json:"mode" enum:"none,local,proxy" doc:"Sign-in mode to store"`
	ProxyHeader    string     `json:"proxyHeader,omitempty" maxLength:"255" doc:"Proxy identity header (proxy mode; default Remote-User)"`
	TrustedProxies string     `json:"trustedProxies,omitempty" maxLength:"4096" doc:"Comma-separated trusted proxy IP addresses or CIDRs (proxy mode)"`
}

// SetupIdentityRequest claims a pending hub: it stores the sign-in mode and creates the first administrator.
type SetupIdentityRequest struct {
	SignInPolicyInput
	Code        string  `json:"code" minLength:"1" maxLength:"64" doc:"One-time setup code from the hub log"`
	Username    string  `json:"username" minLength:"1" maxLength:"255" doc:"Username of the first administrator"`
	DisplayName *string `json:"displayName,omitempty" maxLength:"255" doc:"Display name of the first administrator"`
	Password    *string `json:"password,omitempty" maxLength:"1024" doc:"Password; required in local mode"`
}

// SignInUpdateRequest changes the stored sign-in policy at runtime.
type SignInUpdateRequest struct {
	SignInPolicyInput
	Password *string `json:"password,omitempty" maxLength:"1024" doc:"Sets the caller's password; required when switching to local without one"`
}
