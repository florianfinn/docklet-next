// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run claim and sign-in policy endpoints (docs/design/first-run-setup.md).

package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
)

type SetupIdentityInput struct {
	UserAgent string `header:"User-Agent"`
	Body      auth.SetupIdentityRequest
	request   *http.Request
}

// Resolve keeps the raw request: the proxy check needs the direct peer and the identity header.
func (i *SetupIdentityInput) Resolve(ctx huma.Context) []error {
	i.request = humaecho.Unwrap(ctx).Request()
	return nil
}

type SignInUpdateInput struct {
	UserAgent string `header:"User-Agent"`
	Body      auth.SignInUpdateRequest
	request   *http.Request
}

func (i *SignInUpdateInput) Resolve(ctx huma.Context) []error {
	i.request = humaecho.Unwrap(ctx).Request()
	return nil
}

func registerSetupRoutesInternal(api huma.API, h *AuthHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "claim-setup-identity",
		Method:      http.MethodPost,
		Path:        "/setup/identity",
		Summary:     "Create the first administrator",
		Description: "While first-run setup is pending, store the sign-in mode and create the first administrator. Requires the setup code from the hub log, hub Origin and the CSRF pair; no credential exemption applies. Local mode signs the browser in.",
		Tags:        []string{"Auth"},
		Security:    []map[string][]string{},
	}, h.ClaimSetupIdentity)

	huma.Register(api, huma.Operation{
		OperationID: "get-sign-in",
		Method:      http.MethodGet,
		Path:        "/auth/sign-in",
		Summary:     "Get the sign-in policy",
		Description: "Read the active sign-in mode with its proxy settings and whether AUTH_MODE fixes it.",
		Tags:        []string{"Auth"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, h.GetSignIn)

	huma.Register(api, huma.Operation{
		OperationID: "update-sign-in",
		Method:      http.MethodPut,
		Path:        "/auth/sign-in",
		Summary:     "Change the sign-in mode",
		Description: "Store a new sign-in mode and proxy settings without a restart. Refused while AUTH_MODE fixes the mode, and when the caller would not stay signed in. Revokes every session; in local mode the caller receives a new one.",
		Tags:        []string{"Auth"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, h.UpdateSignIn)
}

type SignInPolicyOutput struct {
	Body base.ApiResponse[auth.SignInConfiguration]
}

func (h *AuthHandler) GetSignIn(ctx context.Context, _ *struct{}) (*SignInPolicyOutput, error) {
	p := h.authService.CurrentPolicy()
	cfg := auth.SignInConfiguration{Mode: p.Mode, SetupPending: p.Pending, Forced: p.Forced, ProxyHeader: &p.Header, TrustedProxies: &p.Trusted}
	if p.Mode == auth.SignInModeProxy && h.authService.config.AuthProxyLogoutURL != "" {
		cfg.LogoutURL = &h.authService.config.AuthProxyLogoutURL
	}
	return &SignInPolicyOutput{Body: base.ApiResponse[auth.SignInConfiguration]{Success: true, Data: cfg}}, nil
}

// policyProblemInternal maps claim and policy errors to typed problems.
func policyProblemInternal(ctx context.Context, err error) error {
	var policyErr *PolicyError
	switch {
	case errors.As(err, &policyErr):
		return huma.Error400BadRequest(policyErr.Err.Error())
	case errors.Is(err, ErrSetupNotPending), errors.Is(err, ErrModeForced), errors.Is(err, ErrProxyUnverified), errors.Is(err, ErrPasswordRequired):
		return access.Problem(http.StatusConflict, err.Error())
	case errors.Is(err, ErrSetupCodeInvalid):
		return access.Problem(http.StatusForbidden, err.Error())
	case errors.Is(err, ErrSetupRateLimited):
		return access.Problem(http.StatusTooManyRequests, err.Error())
	case errors.Is(err, ErrSetupPending):
		return access.Problem(http.StatusUnauthorized, err.Error())
	case errors.Is(err, user.ErrUsernameRequired):
		return huma.Error400BadRequest("A username is required")
	case errors.Is(err, user.ErrUsernameTaken):
		return access.Problem(http.StatusBadRequest, auth.CodeUsernameTaken)
	case err != nil && err.Error() == "Local usernames cannot contain @":
		return huma.Error400BadRequest(err.Error())
	}
	return huma.Error500InternalServerError("Sign-in change failed")
}

func (h *AuthHandler) authenticationOutputInternal(ctx context.Context, result *ClaimResult) (*LoginOutput, error) {
	userResp, err := h.userService.ToUserResponseDto(ctx, *result.User)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map user")
	}
	out := &LoginOutput{Body: base.ApiResponse[auth.AuthenticationResponse]{Success: true, Data: auth.AuthenticationResponse{
		Success: true,
		Status:  auth.AuthenticationStatusAuthenticated,
		User:    &userResp,
	}}}
	if result.Tokens == nil {
		out.SetCookie = cookie.BuildClearTokenCookieStringsFor(cookie.SecureCookieFromContext(ctx))
		return out, nil
	}
	maxAge := max(int(time.Until(result.Tokens.ExpiresAt).Seconds()), 0) + 60
	expiresAt := result.Tokens.ExpiresAt
	out.SetCookie = cookie.BuildTokenCookieStringFor(maxAge, result.Tokens.BrowserToken, cookie.SecureCookieFromContext(ctx))
	out.Body.Data.Token = result.Tokens.AccessToken
	out.Body.Data.RefreshToken = result.Tokens.RefreshToken
	out.Body.Data.ExpiresAt = &expiresAt
	return out, nil
}

func (h *AuthHandler) ClaimSetupIdentity(ctx context.Context, input *SetupIdentityInput) (*LoginOutput, error) {
	result, err := h.authService.ClaimSetup(ctx, input.request, input.Body, handlerutil.SessionMetaFromContext(ctx, input.UserAgent))
	if err != nil {
		return nil, policyProblemInternal(ctx, err)
	}
	return h.authenticationOutputInternal(ctx, result)
}

func (h *AuthHandler) UpdateSignIn(ctx context.Context, input *SignInUpdateInput) (*LoginOutput, error) {
	callerID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("Sign-in required")
	}
	result, err := h.authService.UpdateSignIn(ctx, input.request, callerID, input.Body, handlerutil.SessionMetaFromContext(ctx, input.UserAgent))
	if err != nil {
		return nil, policyProblemInternal(ctx, err)
	}
	return h.authenticationOutputInternal(ctx, result)
}
