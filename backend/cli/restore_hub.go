// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original offline Hub recovery command for issue #32.

package cli

import (
	"errors"
	"fmt"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/startup"
	"github.com/spf13/cobra"
)

var restoreHubBackupID string

var restoreHubCmd = &cobra.Command{
	Use:   "restore-hub",
	Short: "Restore a trusted local Hub archive before starting HTTP",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if restoreHubBackupID == "" {
			return errors.New("backup ID is required")
		}
		cfg := config.Load()
		identity := &startup.RuntimeIdentityConfig{PUID: cfg.PUID, PGID: cfg.PGID, DockerHost: cfg.DockerHost, DockerConfig: cfg.DockerConfig, ProjectsDirectory: cfg.ProjectsDirectory, BackupDirectory: cfg.BackupDir}
		if err := startup.ApplyRequestedRuntimeIdentity(cmd.Context(), identity); err != nil {
			return err
		}
		if err := hubbackup.OfflineRestore(cmd.Context(), cfg, restoreHubBackupID); err != nil {
			return fmt.Errorf("offline Hub restore: %w", err)
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Hub archive restored; start the Hub and sign in again")
		return err
	},
}

func init() {
	restoreHubCmd.Flags().StringVar(&restoreHubBackupID, "backup-id", "", "Server-owned archive ID from the backup directory")
	rootCmd.AddCommand(restoreHubCmd)
}
