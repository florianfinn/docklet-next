// Source: getarcaneapp/arcane/backend/internal/bootstrap/bootstrap.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed standalone build workshop; functional cut: removed OIDC and passkeys; SQLite-only local persistence.; Uses forward-only database initialization.

package bootstrap

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	httpxtypes "github.com/florianfinn/docklet-next/types/httpx"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/subosito/gotenv"
	"go.getarcane.app/streams/logs"
	"go.getarcane.app/sys/crypto"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/di"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/startup"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

func Bootstrap(ctx context.Context) error {
	if err := gotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg := config.Load()
	if err := cfg.ValidateSignIn(); err != nil {
		return err
	}
	runtimeIdentityCfg := &startup.RuntimeIdentityConfig{
		PUID:              cfg.PUID,
		PGID:              cfg.PGID,
		DockerHost:        cfg.DockerHost,
		DockerConfig:      cfg.DockerConfig,
		ProjectsDirectory: cfg.ProjectsDirectory,
	}
	if !cfg.AgentMode {
		runtimeIdentityCfg.BackupDirectory = cfg.BackupDir
	}
	if err := startup.ApplyRequestedRuntimeIdentity(ctx, runtimeIdentityCfg); err != nil {
		return fmt.Errorf("apply runtime identity: %w", err)
	}
	cfg.DockerConfig = runtimeIdentityCfg.DockerConfig

	SetupSlogLogger(cfg)
	// Tee all slog output into the in-memory ring buffer that powers the diagnostics live log tail.
	slog.SetDefault(slog.New(logs.NewSlogHandler(slog.Default().Handler(), ws.LogBroadcaster())))
	database.SetGormLogger(BuildGormLogger(cfg))
	slog.InfoContext(ctx, "Arcane is starting...", "version", config.Version)
	slog.InfoContext(ctx, "Arcane Identity Configuration", "puid", os.Getuid(), "pgid", os.Getgid())

	appCtx, cancelApp := context.WithCancel(ctx)
	appCtx = utils.WithAppLifecycleContext(appCtx)

	var db *database.DB
	var err error
	if cfg.AgentMode {
		db, err = initializeDBAndMigrateInternal(appCtx, cfg)
	} else {
		if markerErr := hubbackup.RecoverPendingRestore(appCtx, cfg); markerErr != nil {
			slog.ErrorContext(appCtx, "Hub restore recovery required; run the offline restore command before restarting the hub", "code", "backup.recovery_required")
			cancelApp()
			return markerErr
		}
		db, err = database.InitializeHub(appCtx, cfg.DatabaseURL, database.MigrationOptions{})
	}
	if err != nil {
		cancelApp()
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	releaseHubLock := true
	defer func() {
		cancelApp()
		closeDatabase := db.CloseRetainingHubLock
		if releaseHubLock {
			closeDatabase = db.Close
		}
		if closeErr := closeDatabase(); closeErr != nil {
			slog.ErrorContext(ctx, "Error closing database", "error", closeErr)
		}
	}()

	app := fx.New(applicationOptions(appCtx, cfg, db, cancelApp))

	startCtx, cancelStart := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelStart()
	if startErr := app.Start(startCtx); startErr != nil {
		return fmt.Errorf("start application: %w", startErr)
	}

	select {
	case <-ctx.Done():
		slog.InfoContext(appCtx, "Context canceled")
	case signal := <-app.Done():
		slog.InfoContext(appCtx, "Received shutdown signal", "signal", signal)
	}

	stopCtx, cancelStop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelStop()
	if stopErr := app.Stop(stopCtx); stopErr != nil {
		releaseHubLock = false
		return fmt.Errorf("stop application: %w", stopErr)
	}

	slog.InfoContext(context.WithoutCancel(appCtx), "Arcane shutdown complete")
	return nil
}

func applicationOptions(appCtx context.Context, cfg *config.Config, db *database.DB, cancelApp context.CancelFunc) fx.Option {
	return fx.Options(
		fx.Supply(cfg, db, cancelApp),
		fx.Provide(
			func() context.Context { return appCtx },
			newConfiguredHTTPClientInternal,
		),
		di.ActorOptions,
		di.ServiceOptions,
		di.JobOptions,
		serverOptions,
		fx.Invoke(
			registerAppCancelHook,
			initializeStartupState,
			registerJobs,
			startEdgeTunnelClient,
		),
		fx.WithLogger(func() fxevent.Logger {
			logger := &fxevent.SlogLogger{Logger: slog.Default()}
			logger.UseLogLevel(slog.LevelDebug)
			return logger
		}),
		fx.StartTimeout(5*time.Minute),
		fx.StopTimeout(30*time.Second),
	)
}

// isWeakProductionEncryptionKeyInternal reports whether an explicit
// ENCRYPTION_KEY is an unprefixed passphrase shorter than 32 characters in
// production. crypto derives a key from any non-empty passphrase, so this
// preserves the historical fail-fast rejection of low-entropy production keys.
func isWeakProductionEncryptionKeyInternal(encryptionKey, localEnvironment string, agentMode bool) bool {
	if localEnvironment != "production" || agentMode {
		return false
	}
	key := strings.TrimSpace(encryptionKey)
	if key == "" || strings.HasPrefix(key, "hex:") || strings.HasPrefix(key, "base64:") {
		return false
	}
	return len(strings.TrimPrefix(key, "raw:")) < 32
}

func warnDeprecatedEnvVarsInternal(ctx context.Context, cfg *config.Config) {
	for _, envName := range cfg.DeprecatedEnvVarsSet() {
		slog.WarnContext(ctx, "Deprecated environment variable is set and no longer used; remove it from your environment", "env", envName)
	}
}

func newConfiguredHTTPClientInternal(cfg *config.Config) *http.Client {
	options := httpxtypes.ClientOptions{
		Timeout:             10 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	if cfg.HTTPClientTimeout > 0 {
		options.Timeout = time.Duration(cfg.HTTPClientTimeout) * time.Second
		options.TLSHandshakeTimeout = 10 * time.Second
	}
	return httpx.NewHTTPClient(options)
}

type initializeStartupStateParams struct {
	fx.In

	AppCtx     context.Context
	Config     *config.Config
	HTTPClient *http.Client

	Volume        *volume.VolumeService
	Settings      *settings.SettingsService
	Environment   *environment.EnvironmentService
	Project       *project.ProjectService
	Docker        *docker.DockerClientService
	Role          *role.RoleService
	User          *user.UserService
	ApiKey        *apikey.ApiKeyService
	Auth          *auth.AuthService
	Vulnerability *vulnerability.VulnerabilityService
}

func initializeStartupState(p initializeStartupStateParams) error {
	appCtx := p.AppCtx
	cfg := p.Config
	httpClient := p.HTTPClient

	if p.Volume != nil {
		startup.CleanupOrphanedVolumeHelpers(appCtx, p.Volume.CleanupOrphanedVolumeHelpers)
	}

	runtimeCfg := &startup.RuntimeConfig{
		AgentMode:         cfg.AgentMode,
		AgentToken:        cfg.AgentToken,
		Environment:       string(cfg.Environment),
		EncryptionKey:     cfg.EncryptionKey,
		AutoLoginUsername: cfg.AutoLoginUsername,
		AdminStaticAPIKey: cfg.AdminStaticAPIKey,
	}

	warnDeprecatedEnvVarsInternal(appCtx, cfg)

	startup.LoadAgentToken(appCtx, runtimeCfg, p.Settings.GetStringSetting)
	startup.EnsureEncryptionKey(appCtx, runtimeCfg, p.Settings.EnsureEncryptionKey)
	cfg.AgentToken = runtimeCfg.AgentToken
	cfg.EncryptionKey = runtimeCfg.EncryptionKey

	if isWeakProductionEncryptionKeyInternal(cfg.EncryptionKey, string(cfg.Environment), cfg.AgentMode) {
		panic("ENCRYPTION_KEY passphrase must be at least 32 characters in production (or use a hex:/base64: encoded 32-byte key)")
	}

	crypto.InitEncryption(&crypto.Config{
		EncryptionKey: cfg.EncryptionKey,
		Environment:   string(cfg.Environment),
		AgentMode:     cfg.AgentMode,
	})
	startup.InitializeDefaultSettings(appCtx, runtimeCfg, p.Settings)

	if err := p.Settings.NormalizeProjectsDirectory(appCtx, cfg.ProjectsDirectory); err != nil {
		slog.WarnContext(appCtx, "Failed to normalize projects directory", "error", err)
	}

	if err := p.Environment.EnsureLocalEnvironment(appCtx, cfg.AppUrl); err != nil {
		slog.WarnContext(appCtx, "Failed to ensure local environment", "error", err)
	}
	p.Vulnerability.ImportLegacyReportFiles(appCtx)
	p.Vulnerability.BackfillScanItems(appCtx)
	if p.Project != nil {
		if err := p.Project.RecoverProjectRenameJournals(appCtx); err != nil {
			slog.WarnContext(appCtx, "Failed to recover interrupted project rename operations on startup", "error", err)
		}
	}

	if !cfg.AgentMode {
		if err := p.Environment.ReconcileEdgeStatusesOnStartup(appCtx); err != nil {
			slog.WarnContext(appCtx, "Failed to reconcile edge environment statuses on startup", "error", err)
		}

	}

	startup.TestDockerConnection(appCtx, func(ctx context.Context) error {
		dockerClient, err := p.Docker.GetClient(ctx)
		if err != nil {
			return err
		}

		version, err := dockerClient.ServerVersion(ctx, client.ServerVersionOptions{})
		if err != nil {
			return err
		}

		effectiveAPIVersion := cmp.Or(strings.TrimSpace(dockerClient.ClientVersion()), strings.TrimSpace(version.APIVersion))
		slog.InfoContext(ctx, "Docker API versions detected", "clientApiVersion", dockerClient.ClientVersion(), "serverApiVersion", version.APIVersion, "effectiveApiVersion", effectiveAPIVersion)
		return nil
	})

	startup.InitializeNonAgentFeatures(appCtx, runtimeCfg,
		p.Role.EnsureBuiltInRoles,
		func(ctx context.Context) error {
			return p.User.CreateDefaultAdmin(ctx)
		},
		func(ctx context.Context) error {
			return p.ApiKey.ReconcileDefaultAdminAPIKey(ctx, runtimeCfg.AdminStaticAPIKey)
		},
		func(ctx context.Context) error {
			startup.InitializeAutoLogin(ctx, runtimeCfg)
			return nil
		},
	)
	startup.CleanupUnknownSettings(appCtx, p.Settings)
	if !cfg.AgentMode {
		if err := p.Auth.InitializeSignIn(appCtx); err != nil {
			return fmt.Errorf("initialize sign-in: %w", err)
		}
	}

	runRoleStartupTasks(appCtx, p.Role, p.ApiKey, cfg, cfg.AgentMode)

	// Auto-pair only applies in Edge mode (where the agent's outbound tunnel is the
	// only path to the manager). Direct mode is passive — the manager dials the agent's
	// HTTP server on TCP 3553, and the manager-side health-check promotes the env to
	// Online once reachability is confirmed.
	if cfg.AgentMode && cfg.EdgeAgent && cfg.AgentToken != "" && cfg.ManagerApiUrl != "" {
		if err := handleAgentBootstrapPairing(appCtx, cfg, httpClient); err != nil {
			slog.WarnContext(appCtx, "Failed to auto-pair agent with manager", "error", err)
		}
	} else if cfg.AgentMode && !cfg.EdgeAgent {
		slog.InfoContext(appCtx, "Direct mode active: agent operates as a passive HTTP server; no outbound connection to manager required")
	}
	return nil
}

func runRoleStartupTasks(ctx context.Context, roleService *role.RoleService, apiKeyService *apikey.ApiKeyService, cfg *config.Config, agentMode bool) {
	if roleService == nil {
		return
	}
	if err := roleService.EnsureBuiltInRoles(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to reconcile built-in roles", "error", err)
	}
	if apiKeyService != nil {
		if err := apiKeyService.BackfillApiKeyPermissions(ctx); err != nil {
			slog.WarnContext(ctx, "Failed to backfill API key permissions", "error", err)
		}
	}
	if agentMode {
		return
	}
	if err := roleService.AssertGlobalAdminExists(ctx); err != nil {
		slog.ErrorContext(ctx, "RBAC global admin guard failed", "error", err)
	}
}

func startEdgeTunnelClient(appCtx context.Context, lc fx.Lifecycle, cfg *config.Config, router *echo.Echo, _ *http.Server) {
	var stop func(context.Context) error
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var startErr error
			stop, startErr = startEdgeTunnelClientIfConfigured(appCtx, cfg, router)
			if startErr != nil {
				slog.ErrorContext(appCtx, "Failed to start edge tunnel client", "error", startErr)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if stop == nil {
				return nil
			}
			return stop(ctx)
		},
	})
}

func registerAppCancelHook(lc fx.Lifecycle, cancelApp context.CancelFunc) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			cancelApp()
			return nil
		},
	})
}

func startEdgeTunnelClientIfConfigured(appCtx context.Context, cfg *config.Config, router http.Handler) (func(context.Context) error, error) {
	managerEndpointConfigured := cfg.ManagerApiUrl != ""
	if !cfg.EdgeAgent || !managerEndpointConfigured || cfg.AgentToken == "" {
		return nil, nil
	}

	edgeCfg := &edge.Config{
		EdgeAgent:             cfg.EdgeAgent,
		EdgeTransport:         cfg.EdgeTransport,
		EdgeReconnectInterval: cfg.EdgeReconnectInterval,
		EdgeMTLSMode:          cfg.EdgeMTLSMode,
		EdgeMTLSCAFile:        cfg.EdgeMTLSCAFile,
		EdgeMTLSCertFile:      cfg.EdgeMTLSCertFile,
		EdgeMTLSKeyFile:       cfg.EdgeMTLSKeyFile,
		EdgeMTLSServerName:    cfg.EdgeMTLSServerName,
		EdgeMTLSAssetsDir:     cfg.EdgeMTLSAssetsDir,
		AppURL:                cfg.GetAppURL(),
		ManagerApiUrl:         cfg.ManagerApiUrl,
		AgentToken:            cfg.AgentToken,
		Port:                  cfg.Port,
		Listen:                cfg.Listen,
	}

	slog.InfoContext(appCtx, "Starting edge agent session client", edge.StartupLogAttrs(edgeCfg)...)
	stop, err := edge.StartTunnelClient(appCtx, edgeCfg, router)
	if err != nil {
		return nil, fmt.Errorf("failed to start edge tunnel client: %w", err)
	}

	slog.InfoContext(appCtx, "Edge tunnel client started", "managerUrl", cfg.ManagerApiUrl)
	return stop, nil
}

func handleAgentBootstrapPairing(ctx context.Context, cfg *config.Config, httpClient *http.Client) error {
	slog.InfoContext(ctx, "Agent mode detected with token, attempting auto-pairing", "managerUrl", cfg.ManagerApiUrl)

	pairURL := strings.TrimRight(httpx.ManagerBaseURL(cfg.ManagerApiUrl), "/") + "/api/environments/pair"

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, pairURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create pairing request: %w", err)
	}

	req.Header.Set("X-Api-Key", cfg.AgentToken)

	if cfg.EdgeAgent && strings.TrimSpace(cfg.ManagerApiUrl) != "" {
		edgeClient, edgeErr := edge.NewManagerHTTPClient(&edge.Config{
			ManagerApiUrl:      cfg.ManagerApiUrl,
			EdgeMTLSMode:       cfg.EdgeMTLSMode,
			EdgeMTLSCAFile:     cfg.EdgeMTLSCAFile,
			EdgeMTLSCertFile:   cfg.EdgeMTLSCertFile,
			EdgeMTLSKeyFile:    cfg.EdgeMTLSKeyFile,
			EdgeMTLSServerName: cfg.EdgeMTLSServerName,
			EdgeMTLSAssetsDir:  cfg.EdgeMTLSAssetsDir,
		}, 10*time.Second)
		if edgeErr != nil {
			return fmt.Errorf("failed to configure edge pairing client: %w", edgeErr)
		}
		httpClient = edgeClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pairing request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		slog.InfoContext(ctx, "Successfully paired agent with manager", "managerUrl", cfg.ManagerApiUrl)
		return nil
	case http.StatusBadRequest:
		// Environment is not in pending status - already paired, this is fine
		if strings.Contains(string(body), "not in pending status") {
			slog.InfoContext(ctx, "Agent already paired with manager", "managerUrl", cfg.ManagerApiUrl)
			return nil
		}
		return fmt.Errorf("pairing failed with status %d: %s", resp.StatusCode, string(body))
	case http.StatusUnauthorized:
		// Invalid API key - could be already paired with a different key, or key was deleted
		// This is not fatal; the agent can still function if it has a valid token configured
		slog.DebugContext(ctx, "Pairing skipped - API key not recognized (agent may already be paired)", "managerUrl", cfg.ManagerApiUrl)
		return nil
	default:
		return fmt.Errorf("pairing failed with status %d: %s", resp.StatusCode, string(body))
	}
}

func initializeDBAndMigrateInternal(ctx context.Context, cfg *config.Config) (*database.DB, error) {
	db, err := database.Initialize(ctx, cfg.DatabaseURL, database.MigrationOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	slog.InfoContext(ctx, "Database initialized successfully")
	return db, nil
}
