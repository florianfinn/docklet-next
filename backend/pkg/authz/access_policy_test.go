// Source: getarcaneapp/arcane/backend/pkg/authz/access_policy_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; omit two tests and their helpers requiring the excluded Svelte source tree.

package authz_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func TestAccessSurfaceRegistryDefinesSettingsCustomizeAndLandingSemantics(t *testing.T) {
	webhooks := requireAccessSurfaceInternal(t, "settings.category.webhooks")
	require.Equal(t, authz.AccessSurfaceKindSettingsCategory, webhooks.Kind)
	require.Equal(t, "/settings/webhooks", webhooks.URL)
	require.Equal(t, authz.AccessModePermissions, webhooks.AccessMode)
	require.Equal(t, authz.AccessMatchModeAnyOf, webhooks.MatchMode)
	require.Equal(t, authz.AccessScopeModeSelectedEnvPlusGlobal, webhooks.ScopeMode)
	require.ElementsMatch(t, []string{authz.PermWebhooksList}, webhooks.Permissions)

	apiKeys := requireAccessSurfaceInternal(t, "settings.category.apikeys")
	require.Equal(t, authz.AccessScopeModeGlobalOnly, apiKeys.ScopeMode)
	require.ElementsMatch(t, []string{authz.PermApiKeysList, authz.PermApiKeysRead}, apiKeys.Permissions)

	jobSchedule := requireAccessSurfaceInternal(t, "settings.category.jobschedule")
	require.Equal(t, authz.AccessScopeModeSelectedEnvPlusGlobal, jobSchedule.ScopeMode)
	require.Equal(t, "/settings/jobs", jobSchedule.URL)
	require.ElementsMatch(t, []string{authz.PermJobsManage}, jobSchedule.Permissions)

	notifications := requireAccessSurfaceInternal(t, "settings.category.notifications")
	require.Equal(t, authz.AccessScopeModeGlobalOnly, notifications.ScopeMode)
	require.ElementsMatch(t, []string{authz.PermNotificationsManage}, notifications.Permissions)

	templates := requireAccessSurfaceInternal(t, "customize.category.templates")
	require.Equal(t, authz.AccessSurfaceKindCustomizeCategory, templates.Kind)
	require.Equal(t, authz.AccessScopeModeGlobalOnly, templates.ScopeMode)
	require.ElementsMatch(t, []string{authz.PermCustomizeManage, authz.PermTemplatesList, authz.PermTemplatesRead}, templates.Permissions)

	variables := requireAccessSurfaceInternal(t, "customize.category.variables")
	require.Equal(t, authz.AccessSurfaceKindCustomizeCategory, variables.Kind)
	require.Equal(t, authz.AccessScopeModeGlobalOnly, variables.ScopeMode)
	require.Equal(t, authz.AccessMatchModeAnyOf, variables.MatchMode)
	require.ElementsMatch(t, []string{authz.PermVariablesRead}, variables.Permissions)
	require.NotContains(t, variables.Permissions, authz.PermTemplatesRead)

	activities := requireAccessSurfaceInternal(t, "route.activities")
	require.Equal(t, authz.AccessSurfaceKindRoute, activities.Kind)
	require.Empty(t, activities.URL)
	require.Equal(t, authz.AccessScopeModeAnyEffectiveScope, activities.ScopeMode)
	require.ElementsMatch(t, []string{authz.PermActivitiesRead}, activities.Permissions)

	oidcRoleMappings := requireAccessSurfaceInternal(t, "route.oidc-role-mappings")
	require.Equal(t, authz.AccessSurfaceKindRoute, oidcRoleMappings.Kind)
	require.Empty(t, oidcRoleMappings.URL)
	require.Equal(t, authz.AccessScopeModeGlobalOnly, oidcRoleMappings.ScopeMode)
	require.Equal(t, authz.AccessMatchModeAllOf, oidcRoleMappings.MatchMode)
	require.ElementsMatch(t, authz.AllPermissions(), oidcRoleMappings.Permissions)

	settingsLanding := requireAccessSurfaceInternal(t, "landing.settings")
	require.Equal(t, authz.AccessSurfaceKindLanding, settingsLanding.Kind)
	require.Equal(t, "/settings", settingsLanding.URL)
	require.Equal(t, authz.AccessModeAnyChild, settingsLanding.AccessMode)
	require.Contains(t, settingsLanding.Children, "settings.category.webhooks")
	require.Contains(t, settingsLanding.Children, "settings.category.apikeys")
	require.Contains(t, settingsLanding.Children, "settings.category.jobschedule")

	dashboard := requireAccessSurfaceInternal(t, "route.dashboard")
	require.Equal(t, authz.AccessSurfaceKindRoute, dashboard.Kind)
	require.Equal(t, "/dashboard", dashboard.URL)
	require.Positive(t, dashboard.FallbackOrder)
}

func TestCanAccessSurfaceEvaluatesScopeModesAndLandingChildren(t *testing.T) {
	ps := authz.NewPermissionSet()
	ps.AddEnv("env-a", authz.PermWebhooksList)

	require.True(t, authz.CanAccessSurface(ps, "settings.category.webhooks", "env-a"))
	require.True(t, authz.CanAccessSurface(ps, "landing.settings", "env-a"))
	require.False(t, authz.CanAccessSurface(ps, "settings.category.webhooks", "env-b"))
	require.False(t, authz.CanAccessSurface(ps, "landing.settings", "env-b"))

	ps.AddGlobal(authz.PermApiKeysRead)
	require.True(t, authz.CanAccessSurface(ps, "settings.category.apikeys", "env-b"))
	require.True(t, authz.CanAccessSurface(ps, "landing.settings", "env-b"))

	jobsPS := authz.NewPermissionSet()
	jobsPS.AddEnv("env-a", authz.PermJobsManage)
	require.True(t, authz.CanAccessSurface(jobsPS, "settings.category.jobschedule", "env-a"))
	require.True(t, authz.CanAccessSurface(jobsPS, "landing.settings", "env-a"))
	require.False(t, authz.CanAccessSurface(jobsPS, "settings.category.jobschedule", "env-b"))

	notificationPS := authz.NewPermissionSet()
	notificationPS.AddEnv("env-a", authz.PermNotificationsManage)
	require.False(t, authz.CanAccessSurface(notificationPS, "settings.category.notifications", "env-a"))
	notificationPS.AddGlobal(authz.PermNotificationsManage)
	require.True(t, authz.CanAccessSurface(notificationPS, "settings.category.notifications", "env-a"))

	customizePS := authz.NewPermissionSet()
	customizePS.AddGlobal(authz.PermCustomizeManage)
	require.True(t, authz.CanAccessSurface(customizePS, "customize.category.registries", "env-a"))
	require.True(t, authz.CanAccessSurface(customizePS, "landing.customize", "env-a"))

	variablePS := authz.NewPermissionSet()
	variablePS.AddGlobal(authz.PermVariablesRead)
	require.True(t, authz.CanAccessSurface(variablePS, "customize.category.variables", "env-a"))
	require.True(t, authz.CanAccessSurface(variablePS, "landing.customize", "env-a"))
	templatePS := authz.NewPermissionSet()
	templatePS.AddGlobal(authz.PermTemplatesRead)
	require.False(t, authz.CanAccessSurface(templatePS, "customize.category.variables", "env-a"))

	activitiesPS := authz.NewPermissionSet()
	activitiesPS.AddEnv("env-a", authz.PermActivitiesRead)
	require.True(t, authz.CanAccessSurface(activitiesPS, "route.activities", "env-b"))
	require.False(t, authz.CanAccessSurface(authz.NewPermissionSet(), "route.activities", "env-a"))

	globalAdminPS := authz.NewPermissionSet()
	globalAdminPS.AddGlobal(authz.AllPermissions()...)
	require.True(t, authz.CanAccessSurface(globalAdminPS, "route.oidc-role-mappings", "env-a"))
	require.True(t, authz.CanAccessSurface(authz.SudoPermissionSet(), "route.oidc-role-mappings", "env-a"))

	nonAdminPS := authz.NewPermissionSet()
	nonAdminPS.AddGlobal(authz.BuiltInEditorPermissions()...)
	require.False(t, authz.CanAccessSurface(nonAdminPS, "route.oidc-role-mappings", "env-a"))

	environmentAdminPS := authz.NewPermissionSet()
	environmentAdminPS.AddEnv("env-a", authz.AllPermissions()...)
	require.False(t, authz.CanAccessSurface(environmentAdminPS, "route.oidc-role-mappings", "env-a"))

	require.False(t, authz.CanAccessSurface(authz.NewPermissionSet(), "missing.surface", "env-a"))
}

func TestAccessSurfaceRegistryIsDefensiveAndInternallyConsistent(t *testing.T) {
	surfaces := authz.AccessSurfaces()
	require.NotEmpty(t, surfaces)

	surfaces[0].Permissions = append(surfaces[0].Permissions, "unknown:permission")
	surfaces[0].Children = append(surfaces[0].Children, "missing.child")

	fresh := authz.AccessSurfaces()
	require.NotContains(t, fresh[0].Permissions, "unknown:permission")
	require.NotContains(t, fresh[0].Children, "missing.child")

	for _, surface := range fresh {
		for _, perm := range surface.Permissions {
			require.True(t, authz.IsKnownPermission(perm), "surface %s references unknown permission %s", surface.ID, perm)
		}
		for _, childID := range surface.Children {
			_, ok := findAccessSurfaceInternal(childID)
			require.True(t, ok, "surface %s references unknown child %s", surface.ID, childID)
		}
	}
}

func TestAccessSurfaceCategoryRegistryCoversBackendCategories(t *testing.T) {
	hiddenSettingsCategories := map[string]struct{}{
		"security": {},
	}

	for _, category := range settings.NewSettingsSearchService().GetSettingsCategories() {
		if _, hidden := hiddenSettingsCategories[category.ID]; hidden {
			continue
		}
		_, ok := findAccessSurfaceInternal("settings.category." + category.ID)
		require.True(t, ok, "settings category %s must have an access surface", category.ID)
	}

	for _, category := range search.NewCustomizeSearchService().GetCustomizeCategories() {
		_, ok := findAccessSurfaceInternal("customize.category." + category.ID)
		require.True(t, ok, "customize category %s must have an access surface", category.ID)
	}
}

func requireAccessSurfaceInternal(t *testing.T, id string) authz.AccessSurface {
	t.Helper()

	surface, ok := findAccessSurfaceInternal(id)
	require.True(t, ok, "expected access surface %s to exist", id)

	return surface
}

func findAccessSurfaceInternal(id string) (authz.AccessSurface, bool) {
	for _, surface := range authz.AccessSurfaces() {
		if surface.ID == id {
			return surface, true
		}
	}
	return authz.AccessSurface{}, false
}
