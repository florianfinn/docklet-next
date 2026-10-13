// Source: getarcaneapp/arcane/cli/pkg/admin/cmd.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; import the user command from its publication-safe directory.

package admin

import (
	"github.com/spf13/cobra"

	"github.com/florianfinn/docklet-next/cli/pkg/admin/apikeys"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/events"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/federatedcredentials"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/notifications"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/oidcmappings"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/roles"
	"github.com/florianfinn/docklet-next/cli/pkg/admin/usercmd"
	"github.com/florianfinn/docklet-next/cli/pkg/backups"
)

// AdminCmd is the parent command for administrative operations.
var AdminCmd = &cobra.Command{
	Use:     "admin",
	Aliases: []string{"adm"},
	Short:   "Administration & platform management",
}

func init() {
	AdminCmd.AddCommand(backups.BackupsCmd)
	AdminCmd.AddCommand(users.UsersCmd)
	AdminCmd.AddCommand(roles.RolesCmd)
	AdminCmd.AddCommand(oidcmappings.OidcMappingsCmd)
	AdminCmd.AddCommand(apikeys.ApiKeysCmd)
	AdminCmd.AddCommand(federatedcredentials.FederatedCredentialsCmd)
	AdminCmd.AddCommand(events.EventsCmd)
	AdminCmd.AddCommand(notifications.NotificationsCmd)
}
