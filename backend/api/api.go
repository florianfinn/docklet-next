// Source: getarcaneapp/arcane/backend/api/api.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; functional cut: removed template catalogue; functional cut: removed federated credentials; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations; functional cut: removed standalone build workshop; functional cut: removed OIDC and passkeys; agent route allowlist.

package api

import (
	"context"
	"encoding/json/v2"
	"io"
	"maps"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	kit "go.getarcane.app/kit/pkg"
	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/api/handlers"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/appimages"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/dashboard"
	"github.com/florianfinn/docklet-next/backend/internal/diagnostics"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/health"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/port"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

const (
	arcaneTypesPrefix = "github.com/florianfinn/docklet-next/types/"
	dockerSDKPrefix   = "github.com/moby/moby"

	// scalarDocsHTML returns the HTML template for Scalar API documentation.
	scalarDocsHTML = `<!doctype html>
<html>
  <head>
    <title>Arcane API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script
      id="api-reference"
      data-url="/api/openapi.json"
      data-configuration='{
        "theme": "purple",
        "darkMode": true,
        "layout": "modern",
        "hiddenClients": ["unirest"],
        "defaultHttpClient": { "targetKey": "shell", "clientKey": "curl" }
      }'></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`
)

var dockerSchemaPrefixes = map[string]string{
	"types":     "DockerTypes",
	"registry":  "DockerRegistry",
	"system":    "DockerSystem",
	"container": "DockerContainer",
	"network":   "DockerNetwork",
	"volume":    "DockerVolume",
	"mount":     "DockerMount",
	"filters":   "DockerFilters",
	"blkiodev":  "DockerBlkiodev",
	"strslice":  "DockerStrslice",
	"events":    "DockerEvents",
	"image":     "DockerImage",
	"swarm":     "DockerSwarm",
}

var jsonV2Format = huma.Format{
	Marshal: func(writer io.Writer, value any) error {
		return json.MarshalWrite(writer, value, jsonV2APIOptions)
	},
	Unmarshal: func(data []byte, value any) error {
		return json.Unmarshal(data, value, jsonV2APIOptions)
	},
}

// customSchemaNamer creates unique schema names using package prefix for types
// from github.com/florianfinn/docklet-next/types to avoid conflicts between packages that have
// types with the same name (e.g., image.Summary vs env.Summary).
func customSchemaNamer(t reflect.Type, hint string) string {
	name := huma.DefaultSchemaNamer(t, hint)
	typeStr := t.String()
	pkgPath := packagePathForType(t)
	shortPkg := shortPackageFromTypeString(typeStr)

	if pkgName, ok := arcanePackageName(pkgPath); ok {
		name = pkgName + name
	} else if dockerPrefix, localOk := dockerSchemaPrefix(pkgPath, shortPkg); localOk {
		name = dockerPrefix + name
	}
	// Preserve the published name after moving the volume backup response to types.
	if pkgPath == arcaneTypesPrefix+"volume" && strings.TrimLeft(typeStr, "*") == "volume.Backup" {
		name = "DockerVolumeVolumeBackup"
	}
	return qualifyGenericArcaneArgumentsInternal(pkgPath, typeStr, name)
}

func packagePathForType(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.PkgPath()
}

func shortPackageFromTypeString(typeStr string) string {
	before, _, ok := strings.Cut(typeStr, ".")
	return kit.Ternary(!ok, "", before)
}

func arcanePackageName(pkgPath string) (string, bool) {
	if !strings.HasPrefix(pkgPath, arcaneTypesPrefix) {
		return "", false
	}

	parts := strings.Split(pkgPath, "/")
	if len(parts) == 0 {
		return "", false
	}

	pkg := parts[len(parts)-1]
	if pkg == "" {
		return "", false
	}

	return kit.Capitalize(pkg), true
}

func dockerSchemaPrefix(pkgPath, shortPkg string) (string, bool) {
	if strings.Contains(pkgPath, dockerSDKPrefix) {
		parts := strings.Split(pkgPath, "/")
		last := parts[len(parts)-1]
		if prefix, ok := dockerSchemaPrefixes[last]; ok {
			return prefix, true
		}
	}

	prefix, ok := dockerSchemaPrefixes[shortPkg]
	if !ok {
		return "", false
	}

	return prefix, true
}

func qualifyGenericArcaneArgumentsInternal(pkgPath, typeName, schemaName string) string {
	openBracket := strings.IndexByte(typeName, '[')
	if !strings.HasPrefix(pkgPath, arcaneTypesPrefix) || openBracket < 0 {
		return schemaName
	}

	outerPackage := strings.TrimPrefix(pkgPath, arcaneTypesPrefix)
	if _, after, found := strings.CutLast(outerPackage, "/"); found {
		outerPackage = after
	}

	plainSchemaName := huma.DefaultSchemaNamer(reflect.TypeFor[struct{}](), typeName)
	schemaPrefix, ok := strings.CutSuffix(schemaName, plainSchemaName)
	if !ok {
		return schemaName
	}

	rewrittenTypeName := typeName
	searchOffset := openBracket + 1
	for {
		prefixIndex := strings.Index(rewrittenTypeName[searchOffset:], arcaneTypesPrefix)
		if prefixIndex == -1 {
			break
		}
		prefixIndex += searchOffset

		afterPrefix := rewrittenTypeName[prefixIndex+len(arcaneTypesPrefix):]
		separator := strings.IndexByte(afterPrefix, '.')
		if separator < 0 {
			break
		}

		innerPackage := afterPrefix[:separator]
		if _, after, found := strings.CutLast(innerPackage, "/"); found {
			innerPackage = after
		}

		replacement := ""
		if innerPackage != outerPackage {
			replacement = kit.Capitalize(innerPackage)
		}

		argumentTypeIndex := prefixIndex + len(arcaneTypesPrefix) + separator + 1
		rewrittenTypeName = rewrittenTypeName[:prefixIndex] + replacement + rewrittenTypeName[argumentTypeIndex:]
		searchOffset = prefixIndex + len(replacement)
	}

	return schemaPrefix + huma.DefaultSchemaNamer(reflect.TypeFor[struct{}](), rewrittenTypeName)
}

// HandlerDeps contains the services required to register HTTP API handlers.
//
// It intentionally contains only dependencies consumed by SetupAPI,
// registerHandlersInternal, and the authentication middleware.
type HandlerDeps struct {
	fx.In

	Runtime           *activity.RuntimeService
	RuntimeHTTP       *activity.RuntimeHTTPService
	RuntimeReceiver   *activity.RuntimeReceiver
	AppImages         *appimages.ApplicationImagesService
	User              *user.Module
	Project           *project.Module
	Environment       *environment.Module
	Settings          *settings.Module
	JobSchedule       *job.Module
	Search            *search.Module
	Container         *container.Module
	Image             *image.Module
	Volume            *volume.Module
	Network           *network.NetworkService
	Port              *port.PortService
	ImageUpdate       *imageupdate.Module
	Auth              *auth.Module
	Docker            *docker.DockerClientService
	ContainerRegistry *registry.Module
	System            *system.Module
	Diagnostics       *diagnostics.DiagnosticsService
	Updater           *updater.Module
	Event             *event.Module
	Activity          *activity.Module
	Version           *version.VersionService
	Notification      *notification.Module
	ApiKey            *apikey.Module
	Webhook           *webhook.Module
	Vulnerability     *vulnerability.Module
	Dashboard         *dashboard.Module
	Role              *role.Module
	Upload            *upload.Module
}

// SetupAPI creates and configures the Huma API attached to the Echo router.
func SetupAPI(e *echo.Echo, apiGroup *echo.Group, appCtx handlerutil.ActivityAppContext, cfg *config.Config, deps HandlerDeps) huma.API {
	e.JSONSerializer = jsonV2Serializer{}

	humaConfig := huma.DefaultConfig("Arcane API", config.Version)
	humaConfig.Formats = maps.Clone(humaConfig.Formats)
	humaConfig.Formats["application/json"] = jsonV2Format
	humaConfig.Formats["json"] = jsonV2Format
	humaConfig.Info.Description = "Modern Docker Management, Designed for Everyone"

	// Disable default docs path - we'll use Scalar instead
	humaConfig.DocsPath = ""

	// Configure servers for OpenAPI spec
	if cfg.AppUrl != "" {
		humaConfig.Servers = []*huma.Server{
			{URL: cfg.AppUrl + "/api"},
		}
	} else {
		humaConfig.Servers = []*huma.Server{
			{URL: "/api"},
		}
	}

	// Configure security schemes
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"BearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "JWT Bearer token authentication",
		},
		"ApiKeyAuth": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-API-Key",
			Description: "API Key authentication",
		},
	}
	humaConfig.Security = []map[string][]string{
		{"BearerAuth": {}},
		{"ApiKeyAuth": {}},
	}
	if cfg != nil && (cfg.AgentMode || cfg.EdgeAgent) {
		humaConfig.OpenAPIPath = ""
		humaConfig.SchemasPath = ""
		humaConfig.DocsPath = ""
	}

	// Use custom schema namer to avoid conflicts between types with same name
	// from different packages (e.g., image.Summary vs env.Summary)
	humaConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", customSchemaNamer)

	// Create Huma API wrapping the Echo router group
	api := humaecho.NewWithGroup(e, apiGroup, humaConfig)

	// Add authentication middleware
	api.UseMiddleware(auth.NewHumaMiddleware(api, deps.Auth.Service(), deps.ApiKey.Service(), deps.Role.Service(), deps.Environment.Service(), cfg))
	api.UseMiddleware(deps.Auth.Service().ViewerMiddleware(api))
	api.UseMiddleware(middleware.NewActivityBatchID())
	registerNormalizationInternal(api)
	registerRuntimeSchemasInternal(api)

	// Register all Huma handlers
	registerHandlersInternal(api, deps, appCtx, cfg)

	// Register Scalar API docs endpoint with dark mode
	if cfg == nil || (!cfg.AgentMode && !cfg.EdgeAgent) {
		registerScalarDocs(apiGroup)
	}

	return api
}

// registerScalarDocs adds the Scalar API documentation endpoint.
func registerScalarDocs(apiGroup *echo.Group) {
	apiGroup.GET("/docs", func(c *echo.Context) error {
		return c.HTML(http.StatusOK, scalarDocsHTML)
	})
}

// SetupAPIForSpec creates a Huma API instance for OpenAPI spec generation only.
// No services are required - this is purely for schema generation.
func SetupAPIForSpec() huma.API {
	spec := setupAPIForSpecMode(nil)
	activity.RegisterRuntimeReceiverRoutes(spec, nil, "")
	return spec
}

func setupAPIForSpecMode(cfg *config.Config) huma.API {
	e := echo.New()
	apiGroup := e.Group("/api")

	humaConfig := huma.DefaultConfig("Arcane API", config.Version)
	humaConfig.Formats = maps.Clone(humaConfig.Formats)
	humaConfig.Formats["application/json"] = jsonV2Format
	humaConfig.Formats["json"] = jsonV2Format
	humaConfig.Info.Description = "Modern Docker Management, Designed for Everyone"
	humaConfig.Servers = []*huma.Server{
		{URL: "/api"},
	}
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"BearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "JWT Bearer token authentication",
		},
		"ApiKeyAuth": {
			Type:        "apiKey",
			In:          "header",
			Name:        "X-API-Key",
			Description: "API Key authentication",
		},
	}
	humaConfig.Security = []map[string][]string{
		{"BearerAuth": {}},
		{"ApiKeyAuth": {}},
	}
	if cfg != nil && (cfg.AgentMode || cfg.EdgeAgent) {
		humaConfig.OpenAPIPath = ""
		humaConfig.SchemasPath = ""
		humaConfig.DocsPath = ""
	}

	// Use custom schema namer to avoid conflicts between types with same name
	humaConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", customSchemaNamer)

	api := humaecho.NewWithGroup(e, apiGroup, humaConfig)

	// Register handlers with zero-value dependencies for schema discovery only.
	registerNormalizationInternal(api)
	registerRuntimeSchemasInternal(api)
	registerHandlersInternal(api, HandlerDeps{}, handlerutil.NewActivityAppContext(context.Background()), cfg) //nolint:forbidigo // Schema discovery has no running request or application lifecycle.

	return api
}

// routeModule is the only registration source for hub, agent and Edge agent APIs.
// Agent modules run with host privileges, so each addition requires review.
type routeModule struct {
	name     string
	agent    bool
	register func(huma.API, HandlerDeps, handlerutil.ActivityAppContext, *config.Config)
}

var routeModules = []routeModule{
	{"runtime", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, cfg *config.Config) {
		if cfg != nil && (cfg.AgentMode || cfg.EdgeAgent) {
			activity.RegisterRuntimeReceiverRoutes(api, d.RuntimeReceiver, "")
		} else {
			activity.RegisterRuntimeHubRoutes(api, d.Runtime)
		}
	}},
	{"health", true, func(api huma.API, _ HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		health.RegisterRoutes(api)
	}},
	{"auth", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Auth.RegisterRoutes(api)
	}},
	{"api-key", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.ApiKey.RegisterRoutes(api)
	}},
	{"role", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Role.RegisterRoutes(api)
	}},
	{"app-images", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		appimages.RegisterAppImages(api, d.AppImages)
	}},
	{"user", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.User.RegisterRoutes(api)
	}},
	{"project", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Project.RegisterRoutes(api, c)
	}},
	{"version", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		version.RegisterVersion(api, d.Version)
	}},
	{"event", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Event.RegisterRoutes(api)
	}},
	{"capabilities", false, func(api huma.API, _ HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		activity.RegisterCapabilities(api)
	}},
	{"activity", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Activity.RegisterRoutes(api)
	}},
	{"environment", false, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Environment.RegisterRoutes(api, c)
	}},
	{"registry", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, cfg *config.Config) {
		if cfg != nil && (cfg.AgentMode || cfg.EdgeAgent) {
			d.ContainerRegistry.RegisterAgentRoutes(api)
		} else {
			d.ContainerRegistry.RegisterRoutes(api)
		}
	}},
	{"image", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Image.RegisterRoutes(api, c)
	}},
	{"upload", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Upload.RegisterRoutes(api)
	}},
	{"image-update", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.ImageUpdate.RegisterRoutes(api, c)
	}},
	{"settings", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Settings.RegisterRoutes(api)
	}},
	{"job", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.JobSchedule.RegisterRoutes(api)
	}},
	{"volume", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Volume.RegisterRoutes(api, c)
	}},
	{"container", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Container.RegisterRoutes(api, c)
	}},
	{"port", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		port.RegisterPorts(api, d.Port)
	}},
	{"network", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		network.RegisterNetworks(api, d.Network, d.Docker, d.Activity.Service(), c)
	}},
	{"notification", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Notification.RegisterRoutes(api)
	}},
	{"updater", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Updater.RegisterRoutes(api, c)
	}},
	{"search", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Search.RegisterRoutes(api)
	}},
	{"system", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, cfg *config.Config) {
		if cfg != nil && (cfg.AgentMode || cfg.EdgeAgent) {
			d.System.RegisterAgentRoutes(api, c)
		} else {
			d.System.RegisterRoutes(api, c)
		}
	}},
	{"diagnostics", true, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		handlers.RegisterDiagnostics(api, d.Diagnostics)
	}},
	{"webhook", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Webhook.RegisterRoutes(api)
	}},
	{"vulnerability", true, func(api huma.API, d HandlerDeps, c handlerutil.ActivityAppContext, _ *config.Config) {
		d.Vulnerability.RegisterRoutes(api, c)
	}},
	{"dashboard", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		d.Dashboard.RegisterRoutes(api)
	}},
	{"stream", false, func(api huma.API, d HandlerDeps, _ handlerutil.ActivityAppContext, _ *config.Config) {
		handlers.RegisterStream(api, d.Dashboard.Handler(), d.Activity.Handler(), d.Environment.Handler(), d.Event.Service(), d.Version)
	}},
}

func registeredModulesForMode(cfg *config.Config) []routeModule {
	agent := cfg != nil && (cfg.AgentMode || cfg.EdgeAgent)
	modules := make([]routeModule, 0, len(routeModules))
	for _, module := range routeModules {
		if !agent || module.agent {
			modules = append(modules, module)
		}
	}
	return modules
}

func registerHandlersInternal(api huma.API, deps HandlerDeps, handlerAppCtx handlerutil.ActivityAppContext, cfg *config.Config) {
	for _, module := range registeredModulesForMode(cfg) {
		module.register(api, deps, handlerAppCtx, cfg)
	}
}
