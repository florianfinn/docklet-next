// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original lockout, rollback, recovery and stream-lifetime tests for the first-run claim and runtime sign-in policy.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

func claimNone(t *testing.T, f *setupFixture) *user.User {
	t.Helper()
	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil), authtypes.SessionMeta{})
	require.NoError(t, err)
	return result.User
}

// restartAuth starts a new auth service over the same database, as a hub restart does.
func restartAuth(f *setupFixture, cfg *config.Config) (*AuthService, error) {
	svc := NewAuthService(f.users, f.svc.settingsService, nil, session.NewSessionService(f.db), cfg).WithSigningKey(f.svc.signingKey)
	svc.browserSigningKey = f.svc.browserSigningKey
	return svc, svc.InitializeSignIn(context.Background())
}

func addAdministrator(t *testing.T, f *setupFixture, name string, created time.Time, disabled bool) *user.User {
	t.Helper()
	u, err := f.users.CreateUser(t.Context(), &user.User{Username: name})
	require.NoError(t, err)
	require.NoError(t, f.roles.SetUserAssignments(t.Context(), u.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
	update := map[string]any{"created_at": created}
	if disabled {
		update["disabled_at"] = created
	}
	require.NoError(t, f.db.Model(&user.User{}).Where("id = ?", u.ID).Updates(update).Error)
	return u
}

func TestSignInProxyChangeRequiresTheCallerPrincipal(t *testing.T) {
	for _, allowlist := range []*string{nil, new("proxy-operator")} {
		name := "unset"
		if allowlist != nil {
			name = "listed"
		}
		t.Run(name, func(t *testing.T) {
			f := newSetupFixture(t, &config.Config{AuthProxyAdminUsers: allowlist}, nil)
			first, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "first-admin", new(setupTestPassword)), authtypes.SessionMeta{})
			require.NoError(t, err)
			second := addAdministrator(t, f, "second-admin", time.Now().Add(time.Hour), false)
			tokens, err := f.svc.CompleteLogin(t.Context(), second, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
			require.NoError(t, err)
			client := newSetupClient(t, f.router)
			client.peer = "192.0.2.10:5000"
			client.headers["Authorization"] = "Bearer " + tokens.AccessToken
			client.headers["X-Example-User"] = "proxy-operator"
			change := client.do(http.MethodPut, "/api/auth/sign-in", authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeProxy, ProxyHeader: "X-Example-User", TrustedProxies: "192.0.2.10"}})
			require.Equal(t, http.StatusConflict, change.Code, "the identity resolves to %s, not the caller %s", first.User.ID, second.ID)
			require.Contains(t, change.Body.String(), authtypes.CodeProxyUnverified)
			require.Equal(t, authtypes.SignInModeLocal, f.svc.CurrentPolicy().Mode)
		})
	}
}

func TestSetupClaimRejectsBlankUsername(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	response := client.do(http.MethodPost, "/api/setup/identity", claimBody(setupCodeOf(f), authtypes.SignInModeNone, "   ", nil))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.True(t, f.svc.CurrentPolicy().Pending)
}

func TestSetupClaimCSRFPairCases(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	body := claimBody(setupCodeOf(f), authtypes.SignInModeNone, "owner", nil)
	valid := client.token
	for name, prepare := range map[string]func(c *setupClient){
		"missing header": func(c *setupClient) { c.token = "" },
		"wrong header":   func(c *setupClient) { c.token = strings.Repeat("x", len(valid)) },
		"missing cookie": func(c *setupClient) { c.cookies = map[string]string{} },
	} {
		attempt := &setupClient{t: t, router: f.router, cookies: map[string]string{}, token: valid, peer: client.peer, headers: map[string]string{}}
		for k, v := range client.cookies {
			attempt.cookies[k] = v
		}
		prepare(attempt)
		require.Equal(t, http.StatusForbidden, attempt.do(http.MethodPost, "/api/setup/identity", body).Code, name)
		require.Equal(t, "", settingValue(t, f.db, session.SettingSetupIdentity), name)
	}
	require.Equal(t, http.StatusOK, client.do(http.MethodPost, "/api/setup/identity", body).Code)
}

func TestSetupClaimRollsBackEveryRowAndRetries(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	code := setupCodeOf(f)
	table := settings.SettingVariable{}.TableName()
	// Fail the last claim write, after the user, role and policy rows.
	require.NoError(t, f.db.Exec("CREATE TRIGGER example_claim_failure BEFORE INSERT ON "+table+" WHEN NEW.key = '"+session.SettingSetupIdentity+"' BEGIN SELECT RAISE(ABORT, 'example failure'); END").Error)
	_, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(code, authtypes.SignInModeLocal, "owner", new(setupTestPassword)), authtypes.SessionMeta{})
	require.Error(t, err)
	var users, assignments int64
	require.NoError(t, f.db.Model(&user.User{}).Count(&users).Error)
	require.NoError(t, f.db.Table("user_role_assignments").Count(&assignments).Error)
	require.Zero(t, users)
	require.Zero(t, assignments)
	for _, key := range []string{session.SettingSetupIdentity, session.SettingAuthMode, "setupCompleted"} {
		require.Equal(t, "", settingValue(t, f.db, key), key)
	}
	require.True(t, f.svc.CurrentPolicy().Pending, "a failed commit publishes nothing")

	require.NoError(t, f.db.Exec("DROP TRIGGER example_claim_failure").Error)
	restarted, err := restartAuth(f, f.svc.config)
	require.NoError(t, err)
	require.True(t, restarted.CurrentPolicy().Pending)
	f.svc = restarted
	_, err = f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "owner", new(setupTestPassword)), authtypes.SessionMeta{})
	require.NoError(t, err)
	_, err = f.svc.AuthenticateLocalPrimary(t.Context(), "owner", setupTestPassword)
	require.NoError(t, err)
}

func TestSetupClaimTakeoverChoosesOldestEnabledThenOldestDisabled(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t.Run("enabled first", func(t *testing.T) {
		var want *user.User
		f := newSetupFixture(t, &config.Config{}, func(f *setupFixture) {
			addAdministrator(t, f, "disabled-oldest", base, true)
			want = addAdministrator(t, f, "enabled-older", base.Add(time.Hour), false)
			addAdministrator(t, f, "enabled-newer", base.Add(2*time.Hour), false)
		})
		claimed := claimNone(t, f)
		require.Equal(t, want.ID, claimed.ID)
	})
	t.Run("only disabled", func(t *testing.T) {
		var want *user.User
		f := newSetupFixture(t, &config.Config{}, func(f *setupFixture) {
			want = addAdministrator(t, f, "disabled-oldest", base, true)
			addAdministrator(t, f, "disabled-newer", base.Add(time.Hour), true)
		})
		claimed := claimNone(t, f)
		require.Equal(t, want.ID, claimed.ID)
		stored, err := f.users.GetUserByID(t.Context(), want.ID)
		require.NoError(t, err)
		require.Nil(t, stored.DisabledAt, "the taken-over administrator is enabled again")
	})
}

func TestSetupCodeIsReplacedOnRestart(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	old := setupCodeOf(f)
	restarted, err := restartAuth(f, f.svc.config)
	require.NoError(t, err)
	f.svc = restarted
	require.NotEqual(t, old, setupCodeOf(f))
	_, err = f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(old, authtypes.SignInModeNone, "owner", nil), authtypes.SessionMeta{})
	require.ErrorIs(t, err, ErrSetupCodeInvalid)
	claimNone(t, f)
}

func TestSignInRuntimeChangeRevokesBearerAndRefresh(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	result, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "owner", new(setupTestPassword)), authtypes.SessionMeta{})
	require.NoError(t, err)
	old := result.Tokens
	_, _, err = f.svc.VerifyToken(t.Context(), old.AccessToken)
	require.NoError(t, err, "the token is cached and valid before the change")
	for _, mode := range []authtypes.SignInMode{authtypes.SignInModeNone, authtypes.SignInModeLocal} {
		_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), result.User.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: mode}}, authtypes.SessionMeta{})
		require.NoError(t, err, mode)
	}
	require.Equal(t, authtypes.SignInModeLocal, f.svc.CurrentPolicy().Mode)
	_, _, err = f.svc.VerifyToken(t.Context(), old.AccessToken)
	require.Error(t, err, "a Bearer token from before local→none→local is revoked")
	_, err = f.svc.RefreshToken(t.Context(), old.RefreshToken, authtypes.SessionMeta{})
	require.Error(t, err, "its refresh token is revoked too")
}

func TestLocalLoginRefusesEmptyPasswordHash(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	client := newSetupClient(t, f.router)
	require.Equal(t, http.StatusOK, client.do(http.MethodPost, "/api/setup/identity", claimBody(setupCodeOf(f), authtypes.SignInModeLocal, "owner", new(setupTestPassword))).Code)
	_, err := f.users.CreateUser(t.Context(), &user.User{Username: "blank"})
	require.NoError(t, err)
	require.True(t, f.svc.CurrentPolicy().LocalActive())
	anonymous := newSetupClient(t, f.router)
	login := anonymous.do(http.MethodPost, "/api/auth/login", map[string]string{"username": "blank", "password": setupTestPassword})
	require.Equal(t, http.StatusUnauthorized, login.Code, "an account without a password never signs in: %s", login.Body.String())
	require.Equal(t, http.StatusOK, anonymous.do(http.MethodPost, "/api/auth/login", map[string]string{"username": "owner", "password": setupTestPassword}).Code)
}

func TestSignInPolicyChangeEndsSessionlessViewers(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	owner := claimNone(t, f)
	viewer := f.svc.ViewerContext(t.Context(), "", owner.ID)
	require.NoError(t, sessionlife.Check(viewer))
	// A proxy-mapped simple user takes the non-administrator binding.
	member, err := f.users.CreateUser(t.Context(), &user.User{Username: "member"})
	require.NoError(t, err)
	memberViewer := f.svc.ViewerContextForPermissions(t.Context(), "", member.ID, &authz.PermissionSet{})
	require.NoError(t, sessionlife.Check(memberViewer))

	proxy := func(header string) authtypes.SignInUpdateRequest {
		return authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeProxy, ProxyHeader: header, TrustedProxies: "192.0.2.10"}}
	}
	viaProxy := func(header string) *http.Request {
		req := httptest.NewRequest(http.MethodPut, "/", nil)
		req.RemoteAddr = "192.0.2.10:5000"
		req.Header.Set(header, "owner")
		return req
	}
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy("X-Example-User"), owner.ID, proxy("X-Example-User"), authtypes.SessionMeta{})
	require.NoError(t, err)
	require.Error(t, sessionlife.Check(viewer), "a none-mode viewer ends with the switch to proxy")
	require.Error(t, sessionlife.Check(memberViewer), "so does a simple user's viewer")

	proxyViewer := f.svc.ViewerContext(t.Context(), "", owner.ID)
	require.NoError(t, sessionlife.Check(proxyViewer))
	_, err = f.svc.UpdateSignIn(t.Context(), viaProxy("X-Other-User"), owner.ID, proxy("X-Other-User"), authtypes.SessionMeta{})
	require.NoError(t, err)
	require.Error(t, sessionlife.Check(proxyViewer), "a same-mode proxy change ends viewers bound to the old header")
}

func TestSignInPolicyChangeClosesOpenWebSocket(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	owner := claimNone(t, f)
	router := echo.New()
	router.GET("/stream", func(c *echo.Context) error {
		conn, err := websocket.Accept(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.CloseNow()
		lease, err := sessionlife.Register(c.Request().Context(), func() { _ = conn.CloseNow() })
		if err != nil {
			return nil
		}
		defer lease.Release()
		for {
			kind, data, err := conn.Read(c.Request().Context())
			if err != nil || sessionlife.Check(c.Request().Context()) != nil {
				return nil
			}
			if err := conn.Write(c.Request().Context(), kind, data); err != nil {
				return nil
			}
		}
	}, NewAuthMiddleware(f.svc, f.svc.config).WithPermissionResolver(f.roles).Add())
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, server.URL+"/stream", nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("before")))
	_, _, err = conn.Read(ctx)
	require.NoError(t, err)
	_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), owner.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}, authtypes.SessionMeta{})
	require.NoError(t, err)
	_ = conn.Write(ctx, websocket.MessageText, []byte("after"))
	_, _, err = conn.Read(ctx)
	require.Error(t, err, "the anonymous stream closes when local sign-in is enabled")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestSetupForcedProxyRemovalKeepsHeaderAndNetworks(t *testing.T) {
	cfg := &config.Config{AuthMode: "proxy", AuthProxyHeader: "X-Example-User", AuthTrustedProxies: "192.0.2.10"}
	f := newSetupFixture(t, cfg, nil)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "192.0.2.10:5000"
	req.Header.Set("X-Example-User", "owner")
	_, err := f.svc.ClaimSetup(t.Context(), req, claimBody(setupCodeOf(f), authtypes.SignInModeProxy, "owner", nil), authtypes.SessionMeta{})
	require.NoError(t, err)
	cfg.AuthMode, cfg.AuthProxyHeader, cfg.AuthTrustedProxies = "", "", ""
	restarted, err := restartAuth(f, cfg)
	require.NoError(t, err)
	p := restarted.CurrentPolicy()
	require.Equal(t, []any{authtypes.SignInModeProxy, "X-Example-User", "192.0.2.10", false}, []any{p.Mode, p.Header, p.Trusted, p.Forced})
}

func TestSignInRecoveryStoresLocalSignIn(t *testing.T) {
	const replacement = "Example-Recovery-Password1!"
	t.Run("unforced", func(t *testing.T) {
		f := newSetupFixture(t, &config.Config{}, nil)
		claimNone(t, f)
		cfg := *f.svc.config
		cfg.AuthResetAdminPassword = replacement
		restarted, err := restartAuth(f, &cfg)
		require.NoError(t, err)
		require.Equal(t, authtypes.SignInModeLocal, restarted.CurrentPolicy().Mode)
		require.Equal(t, "local", settingValue(t, f.db, session.SettingAuthMode))
		_, err = restarted.AuthenticateLocalPrimary(t.Context(), "owner", replacement)
		require.NoError(t, err)
		again, err := restartAuth(f, &cfg)
		require.NoError(t, err)
		require.Equal(t, authtypes.SignInModeLocal, again.CurrentPolicy().Mode, "the stored local sign-in survives removing the value")
	})
	t.Run("forced mode stays", func(t *testing.T) {
		f := newSetupFixture(t, &config.Config{}, nil)
		claimNone(t, f)
		cfg := *f.svc.config
		cfg.AuthMode, cfg.AuthResetAdminPassword = "none", replacement
		restarted, err := restartAuth(f, &cfg)
		require.NoError(t, err)
		require.Equal(t, authtypes.SignInModeNone, restarted.CurrentPolicy().Mode)
		require.Equal(t, "none", settingValue(t, f.db, session.SettingAuthMode))
		admin, err := f.users.GlobalAdministrator(t.Context())
		require.NoError(t, err)
		require.NoError(t, f.users.ValidatePassword(admin.PasswordHash, replacement))
	})
	t.Run("invalid stored policy", func(t *testing.T) {
		f := newSetupFixture(t, &config.Config{}, nil)
		claimNone(t, f)
		require.NoError(t, f.db.Save(&settings.SettingVariable{Key: session.SettingAuthMode, Value: "proxy"}).Error)
		cfg := *f.svc.config
		_, err := restartAuth(f, &cfg)
		require.ErrorContains(t, err, "AUTH_RESET_ADMIN_PASSWORD")
		cfg.AuthResetAdminPassword = replacement
		restarted, err := restartAuth(f, &cfg)
		require.NoError(t, err)
		require.Equal(t, authtypes.SignInModeLocal, restarted.CurrentPolicy().Mode)
		require.Equal(t, "local", settingValue(t, f.db, session.SettingAuthMode))
		_, err = restartAuth(f, &cfg)
		require.NoError(t, err, "the repaired policy starts without the reset value")
	})
}

// A request that authenticated under none mode keeps that policy's lifetime, even when local sign-in
// is published before its viewer binds.
func TestSignInPolicyPublishedDuringAuthenticationEndsTheStream(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	owner := claimNone(t, f)
	type probeKey struct{}
	entered, resume := make(chan struct{}), make(chan struct{})
	var paused atomic.Bool
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("example:pause_auth", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(probeKey{}) == true && paused.CompareAndSwap(false, true) {
			close(entered)
			<-resume
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove("example:pause_auth") })
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), probeKey{}, true)))
			return next(c)
		}
	})
	router.GET("/stream", func(c *echo.Context) error {
		conn, err := websocket.Accept(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.CloseNow()
		lease, err := sessionlife.Register(c.Request().Context(), func() { _ = conn.CloseNow() })
		if err != nil {
			return nil
		}
		defer lease.Release()
		kind, data, err := conn.Read(c.Request().Context())
		if err != nil || sessionlife.Check(c.Request().Context()) != nil {
			return nil
		}
		return conn.Write(c.Request().Context(), kind, data)
	}, NewAuthMiddleware(f.svc, f.svc.config).WithPermissionResolver(f.roles).Add())
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	type dialResult struct {
		conn *websocket.Conn
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/stream", nil)
		done <- dialResult{conn, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("authentication did not reach the barrier")
	}
	_, err := f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), owner.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}, authtypes.SessionMeta{})
	close(resume)
	require.NoError(t, err)
	result := <-done
	if result.err != nil {
		return
	}
	defer result.conn.CloseNow()
	_ = result.conn.Write(ctx, websocket.MessageText, []byte("anonymous after local publication"))
	_, data, err := result.conn.Read(ctx)
	require.Error(t, err, "the stream echoed %q under the later local policy", data)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestMachineStreamsSurvivePolicyPublication(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	token := "example-machine-token"
	resolver := testEnvironmentTokenResolver{env: &environment.Environment{ID: "example", Enabled: true, AccessToken: &token}}
	router := echo.New()
	router.GET("/stream", func(c *echo.Context) error {
		conn, err := websocket.Accept(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer conn.CloseNow()
		lease, err := sessionlife.Register(c.Request().Context(), func() { _ = conn.CloseNow() })
		if err != nil {
			return nil
		}
		defer lease.Release()
		for {
			kind, data, err := conn.Read(c.Request().Context())
			if err != nil || sessionlife.Check(c.Request().Context()) != nil {
				return nil
			}
			if conn.Write(c.Request().Context(), kind, data) != nil {
				return nil
			}
		}
	}, NewAuthMiddleware(f.svc, f.svc.config).WithEnvironmentAccessTokenResolver(resolver).Add())
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var connections []*websocket.Conn
	for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/stream", &websocket.DialOptions{HTTPHeader: http.Header{header: {token}}})
		require.NoError(t, err)
		defer conn.CloseNow()
		connections = append(connections, conn)
	}
	check := func(stage string) {
		for _, conn := range connections {
			require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("machine still open")), stage)
			_, data, err := conn.Read(ctx)
			require.NoError(t, err, stage)
			require.Equal(t, "machine still open", string(data), stage)
		}
	}
	check("pending")
	owner := claimNone(t, f)
	check("after claim")
	_, err := f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), owner.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}, authtypes.SessionMeta{})
	require.NoError(t, err)
	check("after local publication")
}

type snapshotProbeKey struct{}

// pauseAuthentication blocks the first database query of a marked request until resume is closed.
func pauseAuthentication(t *testing.T, f *setupFixture) (entered, resume chan struct{}) {
	t.Helper()
	entered, resume = make(chan struct{}), make(chan struct{})
	var paused atomic.Bool
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("example:pause_huma", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(snapshotProbeKey{}) == true && paused.CompareAndSwap(false, true) {
			close(entered)
			<-resume
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove("example:pause_huma") })
	return entered, resume
}

type viewerProbeOutput struct {
	Body struct {
		Stale bool `json:"stale"`
	}
}

func TestHumaViewerKeepsTheAuthenticationPolicy(t *testing.T) {
	f := newSetupFixture(t, &config.Config{}, nil)
	owner := claimNone(t, f)
	entered, resume := pauseAuthentication(t, f)
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), snapshotProbeKey{}, true)))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(NewHumaMiddleware(api, f.svc, nil, f.svc.userServiceRoleResolver(), nil, f.svc.config))
	api.UseMiddleware(f.svc.ViewerMiddleware(api))
	huma.Register(api, huma.Operation{OperationID: "get-viewer-probe", Method: http.MethodGet, Path: "/viewer-probe", Security: []map[string][]string{{"BearerAuth": {}}}}, func(ctx context.Context, _ *struct{}) (*viewerProbeOutput, error) {
		out := &viewerProbeOutput{}
		out.Body.Stale = sessionlife.Check(ctx) != nil
		return out, nil
	})
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/viewer-probe", nil))
		result <- rec
	}()
	<-entered
	_, err := f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), owner.ID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: new(setupTestPassword)}, authtypes.SessionMeta{})
	close(resume)
	require.NoError(t, err)
	rec := <-result
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"stale":true`, "a viewer bound after the publication belongs to the none policy it authenticated under")
}
