// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run claim and runtime sign-in policy tests (docs/design/first-run-setup.md break map).

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

const setupTestPassword = "Example-Setup-Password1!"

type setupFixture struct {
	svc    *AuthService
	db     *database.DB
	users  *user.UserService
	roles  *role.RoleService
	router *echo.Echo
}

func newSetupFixture(t *testing.T, cfg *config.Config, seed func(*setupFixture)) *setupFixture {
	t.Helper()
	db := setupAuthServiceTestDB(t)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	settingsSvc, err := newSettingsServiceForAuthTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	cfg.JWTRefreshExpiry, cfg.AppUrl, cfg.Environment = time.Hour, "https://hub.example.com", "production"
	f := &setupFixture{db: db, users: users, roles: roles}
	f.svc = NewAuthService(users, settingsSvc, nil, session.NewSessionService(db), cfg).WithSigningKey(newTestSigningKeyInternal())
	f.svc.browserSigningKey = bytes.Repeat([]byte{0x42}, browserSessionSigningKeySize)
	if seed != nil {
		seed(f)
	}
	require.NoError(t, f.svc.InitializeSignIn(t.Context()))
	f.router = setupTestRouter(f.svc, users, settingsSvc)
	return f
}

// setupTestRouter wires the production CSRF, Huma auth middleware and auth routes, plus probes for
// an authenticated Huma operation and an Echo stream route.
func setupTestRouter(svc *AuthService, users *user.UserService, settingsSvc *settings.SettingsService) *echo.Echo {
	router := echo.New()
	authMiddleware := NewAuthMiddleware(svc, svc.config).WithPermissionResolver(svc.userServiceRoleResolver())
	router.Use(middleware.NewCSRFMiddleware(svc.config).
		WithTransportValidator(func(req *http.Request) bool {
			return req.Header.Get(middleware.HeaderAgentToken) == "transport-example"
		}).
		WithLocalCredentialValidator(authMiddleware.ValidateLocalHeaderCredential).
		WithLocalSignInActive(func() bool { return svc.CurrentPolicy().LocalActive() }).Add())
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(NewHumaMiddleware(api, svc, nil, svc.userServiceRoleResolver(), nil, svc.config))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// Production resolves permissions from roles; the claimed administrator is the only user here.
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	RegisterAuth(api, users, svc, settingsSvc)
	huma.Register(api, huma.Operation{OperationID: "get-secure", Method: http.MethodGet, Path: "/secure", Security: []map[string][]string{{"BearerAuth": {}}, {"ApiKeyAuth": {}}}}, func(ctx context.Context, _ *struct{}) (*secureOutput, error) {
		actor, _ := user.CurrentUserFromContext(ctx)
		out := &secureOutput{}
		out.Body.UserID = actor.ID
		return out, nil
	})
	router.GET("/stream/secure", func(c *echo.Context) error { return c.String(http.StatusOK, "open") }, authMiddleware.Add())
	return router
}

type setupClient struct {
	t       *testing.T
	router  *echo.Echo
	cookies map[string]string
	token   string
	peer    string
	headers map[string]string
}

func newSetupClient(t *testing.T, router *echo.Echo) *setupClient {
	c := &setupClient{t: t, router: router, cookies: map[string]string{}, peer: "198.51.100.5:5000", headers: map[string]string{}}
	res := c.do(http.MethodGet, "/api/auth/mode", nil)
	require.Equal(t, http.StatusOK, res.Code)
	c.token = res.Header().Get(middleware.CSRFHeader)
	return c
}

func (c *setupClient) do(method, path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(c.t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = c.peer
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", "https://hub.example.com")
		req.Header.Set(middleware.CSRFHeader, c.token)
	}
	for name, value := range c.headers {
		req.Header.Set(name, value)
	}
	for name, value := range c.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	res := httptest.NewRecorder()
	c.router.ServeHTTP(res, req)
	for _, cookie := range res.Result().Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
	return res
}

func setupCodeOf(f *setupFixture) string {
	f.svc.setup.mu.Lock()
	defer f.svc.setup.mu.Unlock()
	return formatSetupCodeInternal(f.svc.setup.code)
}

func claimBody(code string, mode authtypes.SignInMode, username string, password *string) authtypes.SetupIdentityRequest {
	return authtypes.SetupIdentityRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: mode}, Code: code, Username: username, Password: password}
}

func settingValue(t *testing.T, db *database.DB, key string) string {
	t.Helper()
	var row settings.SettingVariable
	if err := db.Where("key = ?", key).First(&row).Error; err != nil {
		return ""
	}
	return row.Value
}

func TestSetupPendingRefusesCredentialsAndIdentities(t *testing.T) {
	for _, forced := range []string{"", "none", "local"} {
		t.Run("forced="+forced, func(t *testing.T) {
			f := newSetupFixture(t, &config.Config{AuthMode: forced}, nil)
			client := newSetupClient(t, f.router)
			mode := client.do(http.MethodGet, "/api/auth/mode", nil)
			require.Contains(t, mode.Body.String(), `"setupPending":true`)
			require.Equal(t, forced != "", strings.Contains(mode.Body.String(), `"forced":true`))
			login := client.do(http.MethodPost, "/api/auth/login", map[string]string{"username": "arcane", "password": "arcane-admin"})
			require.Equal(t, http.StatusUnauthorized, login.Code)
			require.Contains(t, login.Body.String(), authtypes.CodeSetupPending)
			secure := client.do(http.MethodGet, "/api/secure", nil)
			require.Equal(t, http.StatusUnauthorized, secure.Code)
			require.Contains(t, secure.Body.String(), authtypes.CodeSetupPending)
			stream := client.do(http.MethodGet, "/stream/secure", nil)
			require.Equal(t, http.StatusUnauthorized, stream.Code)
			require.Contains(t, stream.Body.String(), authtypes.CodeSetupPending)
		})
	}
}

func TestSetupClaimLocalCreatesAdministratorAndSession(t *testing.T) {
	var reconciled int
	f := newSetupFixture(t, &config.Config{}, func(f *setupFixture) {
		f.svc.SetAfterClaim(func(context.Context) error { reconciled++; return nil })
	})
	client := newSetupClient(t, f.router)
	code := setupCodeOf(f)
	require.Len(t, code, 14)

	wrong := client.do(http.MethodPost, "/api/setup/identity", claimBody("AAAA-AAAA-AAAA", authtypes.SignInModeLocal, "owner", new(setupTestPassword)))
	require.Equal(t, http.StatusForbidden, wrong.Code)
	require.Contains(t, wrong.Body.String(), authtypes.CodeSetupCodeInvalid)
	noPassword := client.do(http.MethodPost, "/api/setup/identity", claimBody(code, authtypes.SignInModeLocal, "owner", nil))
	require.Equal(t, http.StatusConflict, noPassword.Code)
	require.Contains(t, noPassword.Body.String(), authtypes.CodePasswordRequired)

	claim := client.do(http.MethodPost, "/api/setup/identity", claimBody(strings.ToLower(code), authtypes.SignInModeLocal, "owner", new(setupTestPassword)))
	require.Equal(t, http.StatusOK, claim.Code, claim.Body.String())
	require.Equal(t, 1, reconciled)
	require.NotEmpty(t, client.cookies, "local claim signs the browser in")
	require.Equal(t, http.StatusOK, client.do(http.MethodGet, "/api/secure", nil).Code)
	require.Contains(t, client.do(http.MethodGet, "/api/auth/mode", nil).Body.String(), `"setupPending":false`)
	require.Equal(t, session.SetupIdentityDone, settingValue(t, f.db, session.SettingSetupIdentity))
	require.Equal(t, "false", settingValue(t, f.db, "setupCompleted"))
	require.Equal(t, "local", settingValue(t, f.db, session.SettingAuthMode))

	again := client.do(http.MethodPost, "/api/setup/identity", claimBody(code, authtypes.SignInModeLocal, "other", new(setupTestPassword)))
	require.Equal(t, http.StatusConflict, again.Code)
	require.Contains(t, again.Body.String(), authtypes.CodeSetupNotPending)
	var count int64
	require.NoError(t, f.db.Model(&user.User{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	_, err := f.svc.AuthenticateLocalPrimary(t.Context(), "owner", setupTestPassword)
	require.NoError(t, err)
}

func TestSetupClaimRateLimit(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	for range setupFailureLimit {
		require.Equal(t, http.StatusForbidden, client.do(http.MethodPost, "/api/setup/identity", claimBody("WRONG", authtypes.SignInModeNone, "owner", nil)).Code)
	}
	limited := client.do(http.MethodPost, "/api/setup/identity", claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil))
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.Contains(t, limited.Body.String(), authtypes.CodeSetupRateLimited)
	later := time.Now().Add(2 * setupFailureWindow)
	f.svc.setup.now = func() time.Time { return later }
	require.Equal(t, http.StatusOK, client.do(http.MethodPost, "/api/setup/identity", claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil)).Code)
}

func TestSetupClaimIsStrictCSRF(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	body := claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil)
	client.headers["Origin"] = "https://attacker.example.org"
	require.Equal(t, http.StatusForbidden, client.do(http.MethodPost, "/api/setup/identity", body).Code)
	// A cookie-less request with a credential header gets no exemption either.
	bare := &setupClient{t: t, router: f.router, cookies: map[string]string{}, peer: client.peer, headers: map[string]string{"Authorization": "Bearer example", "Origin": "https://hub.example.com"}}
	require.Equal(t, http.StatusForbidden, bare.do(http.MethodPost, "/api/setup/identity", body).Code)
	transport := &setupClient{t: t, router: f.router, cookies: map[string]string{}, peer: client.peer, headers: map[string]string{middleware.HeaderAgentToken: "transport-example", "Origin": "https://attacker.example.org"}}
	require.Equal(t, http.StatusForbidden, transport.do(http.MethodPost, "/api/setup/identity", body).Code, "transport exemptions do not cover the claim")
	require.Equal(t, "", settingValue(t, f.db, session.SettingSetupIdentity))
}

func TestSetupConcurrentClaimsCreateOneAdministrator(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	code := setupCodeOf(f)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			req := httptest.NewRequest(http.MethodPost, "/api/setup/identity", nil)
			_, errs[i] = f.svc.ClaimSetup(t.Context(), req, claimBody(code, authtypes.SignInModeNone, []string{"first", "second"}[i], nil), authtypes.SessionMeta{})
		})
	}
	wg.Wait()
	succeeded, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrSetupNotPending), errors.Is(err, ErrSetupCodeInvalid):
			refused++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, refused)
	var users []user.User
	require.NoError(t, f.db.Find(&users).Error)
	require.Len(t, users, 1)
}

func TestSetupClaimRechecksInsideTransaction(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	code := setupCodeOf(f)
	// Another writer recorded the claim after this process published its pending snapshot.
	require.NoError(t, f.db.Save(&settings.SettingVariable{Key: session.SettingSetupIdentity, Value: session.SetupIdentityDone}).Error)
	_, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(code, authtypes.SignInModeNone, "late", nil), authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrSetupNotPending)
	var count int64
	require.NoError(t, f.db.Model(&user.User{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestSetupClaimTakesOverUpgradedAdministrator(t *testing.T) {
	var legacy *user.User
	var legacyTokens *TokenPair
	f := newSetupFixture(t, &config.Config{}, func(f *setupFixture) {
		hash, err := f.users.HashPassword("arcane-admin")
		require.NoError(t, err)
		legacy, err = f.users.CreateUser(t.Context(), &user.User{Username: "arcane", PasswordHash: hash, RequiresPasswordChange: true})
		require.NoError(t, err)
		require.NoError(t, f.roles.SetUserAssignments(t.Context(), legacy.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
		other, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
		require.NoError(t, err)
		require.NotEqual(t, legacy.ID, other.ID)
		// A 1.0.0 hub had local sessions and no claim marker.
		f.svc.policy.Store(&Policy{Mode: authtypes.SignInModeLocal, Header: defaultProxyHeader})
		legacyTokens, err = f.svc.CompleteLogin(t.Context(), legacy, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
		require.NoError(t, err)
		f.svc.policy.Store(nil)
	})
	require.True(t, f.svc.CurrentPolicy().Pending)
	_, _, err := f.svc.VerifyToken(t.Context(), legacyTokens.AccessToken)
	require.Error(t, err, "old sessions stop while setup is pending")
	_, err = f.svc.AuthenticateLocalPrimary(t.Context(), "arcane", "arcane-admin")
	require.Error(t, err)

	taken := claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "viewer", new(setupTestPassword))
	_, err = f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), taken, authtypes.SessionMeta{})
	require.ErrorIs(t, err, user.ErrUsernameTaken)

	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "owner", new(setupTestPassword)), authtypes.SessionMeta{})
	require.NoError(t, err)
	require.Equal(t, legacy.ID, result.User.ID, "takeover keeps the administrator ID")
	_, _, err = f.svc.VerifyToken(t.Context(), legacyTokens.AccessToken)
	require.Error(t, err)
	_, err = f.svc.AuthenticateLocalPrimary(t.Context(), "arcane", "arcane-admin")
	require.Error(t, err)
	owner, err := f.svc.AuthenticateLocalPrimary(t.Context(), "owner", setupTestPassword)
	require.NoError(t, err)
	require.False(t, owner.RequiresPasswordChange)
	var count int64
	require.NoError(t, f.db.Model(&user.User{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestSetupForcedModeLocksChoiceAndSurvivesRemoval(t *testing.T) {
	cfg := &config.Config{AuthMode: "none"}
	f := newSetupFixture(t, cfg, nil)
	code := setupCodeOf(f)
	_, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(code, authtypes.SignInModeLocal, "owner", new(setupTestPassword)), authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrModeForced)
	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(code, authtypes.SignInModeNone, "owner", nil), authtypes.SessionMeta{})
	require.NoError(t, err)
	require.Equal(t, "none", settingValue(t, f.db, session.SettingAuthMode), "the claim records the forced mode")
	_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), result.User.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}, authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrModeForced)

	cfg.AuthMode = ""
	require.NoError(t, f.svc.InitializeSignIn(t.Context()))
	require.Equal(t, authtypes.SignInModeNone, f.svc.CurrentPolicy().Mode, "removing AUTH_MODE keeps the recorded mode")
	require.False(t, f.svc.CurrentPolicy().Forced)
}

func TestSignInRuntimeChangeRevokesSessionsAndReissues(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	require.Equal(t, http.StatusOK, client.do(http.MethodPost, "/api/setup/identity", claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "owner", new(setupTestPassword))).Code)
	before := map[string]string{}
	for name, value := range client.cookies {
		before[name] = value
	}

	toNone := client.do(http.MethodPut, "/api/auth/sign-in", authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeNone}})
	require.Equal(t, http.StatusOK, toNone.Code, toNone.Body.String())
	require.Equal(t, authtypes.SignInModeNone, f.svc.CurrentPolicy().Mode)
	require.Equal(t, "none", settingValue(t, f.db, session.SettingAuthMode))
	anonymous := &setupClient{t: t, router: f.router, cookies: map[string]string{}, peer: client.peer, headers: map[string]string{}}
	require.Equal(t, http.StatusOK, anonymous.do(http.MethodGet, "/api/secure", nil).Code, "none mode opens without sign-in")

	toLocal := client.do(http.MethodPut, "/api/auth/sign-in", authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}})
	require.Equal(t, http.StatusOK, toLocal.Code, toLocal.Body.String())
	require.Equal(t, http.StatusOK, client.do(http.MethodGet, "/api/secure", nil).Code, "the caller receives a new session")
	stale := &setupClient{t: t, router: f.router, cookies: before, peer: client.peer, headers: map[string]string{}}
	require.Equal(t, http.StatusUnauthorized, stale.do(http.MethodGet, "/api/secure", nil).Code, "sessions from before the switch are revoked")
	require.Equal(t, http.StatusUnauthorized, anonymous.do(http.MethodGet, "/api/secure", nil).Code)
}

func TestSignInChangeLockoutGuards(t *testing.T) {
	cfg := &config.Config{}
	f := newSetupFixture(t, cfg, nil)
	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil), authtypes.SessionMeta{})
	require.NoError(t, err)
	ownerID := result.User.ID

	toLocal := authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}}
	_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), ownerID, toLocal, authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrPasswordRequired, "a passwordless administrator must set a password")
	_, err = f.svc.AuthenticateLocalPrimary(t.Context(), "owner", "")
	require.Error(t, err)

	proxy := authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeProxy, TrustedProxies: "192.0.2.10", ProxyHeader: "X-Example-User"}}
	direct := httptest.NewRequest(http.MethodPut, "/", nil)
	direct.RemoteAddr = "198.51.100.5:5000"
	direct.Header.Set("X-Example-User", "owner")
	_, err = f.svc.UpdateSignIn(t.Context(), direct, ownerID, proxy, authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrProxyUnverified, "a request outside the trusted list would be locked out")
	viaProxyNoHeader := httptest.NewRequest(http.MethodPut, "/", nil)
	viaProxyNoHeader.RemoteAddr = "192.0.2.10:5000"
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxyNoHeader, ownerID, proxy, authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrProxyUnverified)
	require.Equal(t, authtypes.SignInModeNone, f.svc.CurrentPolicy().Mode, "refused changes leave the policy")

	admins := "someone-else"
	cfg.AuthProxyAdminUsers = &admins
	viaProxy := httptest.NewRequest(http.MethodPut, "/", nil)
	viaProxy.RemoteAddr = "192.0.2.10:5000"
	viaProxy.Header.Set("X-Example-User", "stranger")
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy, ownerID, proxy, authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrProxyUnverified, "an identity that does not resolve to the caller is refused")
	viaProxy.Header.Set("X-Example-User", "owner")
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy, ownerID, proxy, authtypes.SessionMeta{})
	require.NoError(t, err)
	require.Equal(t, authtypes.SignInModeProxy, f.svc.CurrentPolicy().Mode)
	require.Equal(t, "X-Example-User", settingValue(t, f.db, session.SettingAuthProxyHeader))

	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy, ownerID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeProxy, TrustedProxies: "0.0.0.0/0"}}, authtypes.SessionMeta{})
	var policyErr *PolicyError
	require.ErrorAs(t, err, &policyErr)

	withPassword := toLocal
	withPassword.Password = new(setupTestPassword)
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy, ownerID, withPassword, authtypes.SessionMeta{})
	require.NoError(t, err)
	_, err = f.svc.AuthenticateLocalPrimary(t.Context(), "owner", setupTestPassword)
	require.NoError(t, err)
}

func TestSetupPendingIgnoresRecoveryPassword(t *testing.T) {
	cfg := &config.Config{AuthResetAdminPassword: setupTestPassword}
	f := newSetupFixture(t, cfg, nil)
	require.True(t, f.svc.CurrentPolicy().Pending)
	require.Empty(t, cfg.AuthResetAdminPassword, "the recovery value is consumed and ignored while pending")
}

func TestStoredPolicyInvalidFailsStartup(t *testing.T) {
	db := setupAuthServiceTestDB(t)
	require.NoError(t, db.Save(&settings.SettingVariable{Key: session.SettingSetupIdentity, Value: session.SetupIdentityDone}).Error)
	require.NoError(t, db.Save(&settings.SettingVariable{Key: session.SettingAuthMode, Value: "proxy"}).Error)
	users := user.NewUserService(db, role.NewRoleService(db), session.RevokeAllUserSessionsExceptInDB)
	settingsSvc, err := newSettingsServiceForAuthTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	svc := NewAuthService(users, settingsSvc, nil, session.NewSessionService(db), &config.Config{AppUrl: "https://hub.example.com"})
	err = svc.InitializeSignIn(t.Context())
	require.ErrorContains(t, err, "AUTH_RESET_ADMIN_PASSWORD")
}

func TestSignInChangeRollbackKeepsPolicy(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil), authtypes.SessionMeta{})
	require.NoError(t, err)
	require.NoError(t, f.db.Migrator().DropTable(&session.UserSession{}))
	change := authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}
	_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), result.User.ID, change, authtypes.SessionMeta{})
	require.Error(t, err)
	require.Equal(t, authtypes.SignInModeNone, f.svc.CurrentPolicy().Mode, "a failed commit publishes nothing")
	require.Equal(t, "none", settingValue(t, f.db, session.SettingAuthMode))
}
