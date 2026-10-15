// Source: getarcaneapp/arcane/cli/pkg/backups/cmd.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Hub archive list, create, delete and policy contract; volume recovery helpers retained.

package backups

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/spf13/cobra"

	"github.com/florianfinn/docklet-next/cli/internal/client"
	"github.com/florianfinn/docklet-next/cli/internal/cmdutil"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/prompt"
	"github.com/florianfinn/docklet-next/cli/internal/types"
)

var (
	limitFlag  int
	startFlag  int
	allFlag    bool
	forceFlag  bool
	jsonOutput bool

	createPolicyID string

	restoreRecoveryKey string

	policiesUpdateFile      string
	policiesUpdateID        string
	policiesUpdateEnabled   bool
	policiesUpdateDisabled  bool
	policiesUpdateSchedule  string
	policiesUpdateRetention int

	generateSave   bool
	setRecoveryKey string
)

// BackupsCmd is the parent command for Hub database backup operations (admin-only).
var BackupsCmd = &cobra.Command{
	Use:     "backups",
	Aliases: []string{"backup"},
	Short:   "Manage Hub database backups (admin only)",
	Long: `Hub backups archive the PostgreSQL database on a dedicated local volume.
Policies schedule automatic archives.`,
}

var listCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "List Hub database backups",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		return cmdutil.RunList(cmd, c, cmdutil.ListSpec[backup.SystemBackupRun]{
			Resource: "backups",
			Endpoint: types.Backups(),
			Params:   cmdutil.ListParams{Resource: "backups", Limit: limitFlag, FallbackDefault: 20, Start: startFlag, All: allFlag},
			JSON:     cmdutil.JSONOutputEnabled(cmd),
			Headers:  []string{"ID", "STATUS", "TRIGGER", "SIZE", "CREATED"},
			Row: func(run backup.SystemBackupRun) []string {
				size := ""
				if run.Size > 0 {
					size = output.Bytes(run.Size)
				}
				return []string{
					run.ID,
					run.Status,
					run.Trigger,
					size,
					run.CreatedAt.Format("2006-01-02 15:04"),
				}
			},
		})
	},
}

var createCmd = &cobra.Command{
	Use:          "create",
	Short:        "Create a Hub database archive",
	Long:         "Create a local PostgreSQL archive.",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		req := backup.CreateSystemBackupRequest{PolicyID: createPolicyID}

		run, err := c.DoJSON[backup.SystemBackupRun](cmd.Context(), http.MethodPost, types.Backups(), req)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(run)
		}

		output.Success("Backup %s started", run.ID)
		output.KeyValue("ID", run.ID)
		output.KeyValue("Status", run.Status)
		if run.Size > 0 {
			output.KeyValue("Size", output.Bytes(run.Size))
		}
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:          "delete <backup-id>",
	Aliases:      []string{"rm", "remove"},
	Short:        "Delete a Hub database archive",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !forceFlag {
			confirmed, err := cmdutil.Confirm(cmd, fmt.Sprintf("Are you sure you want to delete backup %s?", args[0]))
			if err != nil {
				return err
			}
			if !confirmed {
				_, _ = fmt.Fprintln(output.Stdout(), "Cancelled")
				return nil
			}
		}

		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.DoJSON[backup.SystemBackupOperation](cmd.Context(), http.MethodDelete, types.Backup(args[0]), nil)
		if err != nil {
			return fmt.Errorf("failed to delete backup: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(result)
		}

		output.Success("Backup %s deleted", args[0])
		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:          "restore <backup-id>",
	Short:        "Restore Arcane from a system backup",
	Long:         `Restore overwrites the current Arcane database and configuration with the backup contents, then restarts Arcane.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !forceFlag {
			output.Warning("Restoring backup %s will OVERWRITE the current Arcane database and configuration, then restart Arcane. This cannot be undone.", args[0])
			confirmed, err := cmdutil.Confirm(cmd, "Are you absolutely sure you want to restore this backup?")
			if err != nil {
				return err
			}
			if !confirmed {
				_, _ = fmt.Fprintln(output.Stdout(), "Cancelled")
				return nil
			}
		}

		key := restoreRecoveryKey
		if key == "" {
			var err error
			if key, err = output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Recovery key", "recovery-key"); err != nil {
				return fmt.Errorf("failed to read recovery key: %w", err)
			}
			if key == "" {
				return errors.New("recovery key is required")
			}
		}

		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.PostJSON[base.MessageResponse](cmd.Context(), types.BackupRestore(args[0]), backup.RestoreSystemBackupRequest{RecoveryKey: key})
		if err != nil {
			return fmt.Errorf("failed to restore backup: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(result.Data)
		}

		output.Success("%s", result.Data.Message)
		output.Warning("Arcane will restart while the restore is applied; the server may be unavailable for a short time.")
		return nil
	},
}

var policiesCmd = &cobra.Command{
	Use:          "policies",
	Short:        "Show Hub database backup policies",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.DoJSON[backup.SystemBackupPolicyCollection](cmd.Context(), http.MethodGet, types.BackupsPolicies(), nil)
		if err != nil {
			return fmt.Errorf("failed to get backup policies: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(result)
		}

		return printPolicies(result)
	},
}

func printPolicies(result backup.SystemBackupPolicyCollection) error {
	output.Header("System Backup Policies")

	headers := []string{"ID", "ENABLED", "SCHEDULE", "RETENTION", "LAST RUN"}
	rows := make([][]string, len(result.Policies))
	for i, policy := range result.Policies {
		lastRun := ""
		if policy.LastRun != nil {
			lastRun = fmt.Sprintf("%s (%s)", policy.LastRun.CreatedAt.Format("2006-01-02 15:04"), policy.LastRun.Status)
		}
		rows[i] = []string{
			policy.ID,
			strconv.FormatBool(policy.Enabled),
			policy.Schedule,
			strconv.Itoa(policy.RetentionCount),
			lastRun,
		}
	}
	return output.Table(headers, rows)
}

var policiesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Hub database backup policies",
	Long: `Update a system backup policy with flags: the current policies are fetched, the targeted
policy is modified, and the whole set is saved back. A new policy is created when none exist.
Alternatively, --file replaces all policies from a raw JSON payload.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		var req backup.UpdateSystemBackupPolicies
		if policiesUpdateFile != "" {
			for _, name := range []string{"policy-id", "enabled", "disabled", "schedule", "retention"} {
				if cmd.Flags().Changed(name) {
					return fmt.Errorf("--file cannot be combined with --%s", name)
				}
			}

			var data []byte
			if policiesUpdateFile == "-" {
				data, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read policies from stdin: %w", err)
				}
			} else {
				data, err = os.ReadFile(policiesUpdateFile)
				if err != nil {
					return fmt.Errorf("failed to read file %s: %w", policiesUpdateFile, err)
				}
			}
			if unmarshalErr := json.Unmarshal(data, &req); unmarshalErr != nil {
				return fmt.Errorf("failed to parse policies file: %w", unmarshalErr)
			}
		} else {
			current, getPoliciesErr := c.DoJSON[backup.SystemBackupPolicyCollection](cmd.Context(), http.MethodGet, types.BackupsPolicies(), nil)
			if getPoliciesErr != nil {
				return fmt.Errorf("failed to get current backup policies: %w", getPoliciesErr)
			}

			req.Policies = make([]backup.UpdateSystemBackupPolicy, len(current.Policies))
			targetIndex := -1
			for i, p := range current.Policies {
				req.Policies[i] = backup.UpdateSystemBackupPolicy{
					ID:             p.ID,
					Enabled:        p.Enabled,
					Schedule:       p.Schedule,
					RetentionCount: p.RetentionCount,
				}
				if policiesUpdateID != "" && p.ID == policiesUpdateID {
					targetIndex = i
				}
			}

			switch {
			case policiesUpdateID != "" && targetIndex == -1:
				return fmt.Errorf("policy %q not found; run `arcane admin backups policies`", policiesUpdateID)
			case policiesUpdateID == "" && len(req.Policies) > 1:
				return fmt.Errorf("%d backup policies exist; select one with --policy-id", len(req.Policies))
			case policiesUpdateID == "" && len(req.Policies) == 1:
				targetIndex = 0
			case targetIndex == -1:
				if !cmd.Flags().Changed("schedule") {
					return errors.New("--schedule is required when creating a new backup policy")
				}
				req.Policies = append(req.Policies, backup.UpdateSystemBackupPolicy{})
				targetIndex = len(req.Policies) - 1
			}

			target := &req.Policies[targetIndex]
			if cmd.Flags().Changed("enabled") {
				target.Enabled = true
			}
			if cmd.Flags().Changed("disabled") {
				target.Enabled = false
			}
			if cmd.Flags().Changed("schedule") {
				target.Schedule = policiesUpdateSchedule
			}
			if cmd.Flags().Changed("retention") {
				target.RetentionCount = policiesUpdateRetention
			}
		}

		result, err := c.DoJSON[backup.SystemBackupPolicyCollection](cmd.Context(), http.MethodPut, types.BackupsPolicies(), req)
		if err != nil {
			return fmt.Errorf("failed to update backup policies: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(result)
		}

		output.Success("Backup policies updated successfully")
		return printPolicies(result)
	},
}

var recoveryKeyCmd = &cobra.Command{
	Use:   "recovery",
	Short: "Manage the Arcane system backup recovery key",
}

var recoveryKeyGenerateCmd = &cobra.Command{
	Use:          "generate",
	Short:        "Generate a new Arcane system backup recovery key",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.DoJSON[backup.SystemBackupRecoveryKey](cmd.Context(), http.MethodPost, types.BackupsRecoveryKeyGenerate(), nil)
		if err != nil {
			return fmt.Errorf("failed to generate recovery key: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			if generateSave {
				if storeRecoveryKeyErr := storeRecoveryKey(cmd, c, result.RecoveryKey); storeRecoveryKeyErr != nil {
					return storeRecoveryKeyErr
				}
			}
			return cmdutil.PrintJSON(result)
		}

		output.Header("Generated Recovery Key")
		_, _ = fmt.Fprintln(output.Stdout(), result.RecoveryKey)
		output.Warning("Store this recovery key somewhere safe. Backups encrypted with it cannot be restored without it.")

		save := generateSave
		if !save && prompt.IsInteractive() {
			save, err = cmdutil.Confirm(cmd, "Store this recovery key on the server now?")
			if err != nil {
				return err
			}
		}
		if !save {
			_, _ = fmt.Fprintln(output.Stdout(), "Run `arcane admin backups recovery set` to store it on the server later.")
			return nil
		}
		if storeRotatedRecoveryKeyErr := storeRecoveryKey(cmd, c, result.RecoveryKey); storeRotatedRecoveryKeyErr != nil {
			return storeRotatedRecoveryKeyErr
		}
		output.Success("Recovery key stored on the server")
		return nil
	},
}

var recoveryKeySetCmd = &cobra.Command{
	Use:          "set",
	Short:        "Configure the Arcane system backup recovery key",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		key := setRecoveryKey
		if key == "" {
			var err error
			if key, err = output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Recovery key", "recovery-key"); err != nil {
				return fmt.Errorf("failed to read recovery key: %w", err)
			}
		}
		if key == "" {
			return errors.New("recovery key is required; use --recovery-key")
		}

		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		status, err := c.DoJSON[backup.SystemBackupRecoveryKeyStatus](cmd.Context(), http.MethodPut, types.BackupsRecoveryKey(), backup.SystemBackupRecoveryKey{RecoveryKey: key})
		if err != nil {
			return fmt.Errorf("failed to set recovery key: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(status)
		}

		output.Success("Recovery key configured successfully")
		return nil
	},
}

func storeRecoveryKey(cmd *cobra.Command, c *client.Client, key string) error {
	_, err := c.DoJSON[backup.SystemBackupRecoveryKeyStatus](cmd.Context(), http.MethodPut, types.BackupsRecoveryKey(), backup.SystemBackupRecoveryKey{RecoveryKey: key})
	if err != nil {
		return fmt.Errorf("failed to store recovery key: %w", err)
	}
	return nil
}

func init() {
	BackupsCmd.AddCommand(listCmd)
	BackupsCmd.AddCommand(createCmd)
	BackupsCmd.AddCommand(deleteCmd)
	BackupsCmd.AddCommand(restoreCmd)
	BackupsCmd.AddCommand(policiesCmd)
	BackupsCmd.AddCommand(recoveryKeyCmd)

	policiesCmd.AddCommand(policiesUpdateCmd)
	recoveryKeyCmd.AddCommand(recoveryKeyGenerateCmd)
	recoveryKeyCmd.AddCommand(recoveryKeySetCmd)

	listCmd.Flags().IntVarP(&limitFlag, "limit", "n", 20, "Number of backups to show")
	listCmd.Flags().IntVar(&startFlag, "start", 0, cmdutil.StartFlagUsage)
	listCmd.Flags().BoolVarP(&allFlag, "all", "a", false, cmdutil.AllFlagUsage)
	listCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	createCmd.Flags().StringVar(&createPolicyID, "policy", "", "Backup policy ID to associate with the run")
	createCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	deleteCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Delete without confirmation")
	deleteCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	restoreCmd.Flags().StringVar(&restoreRecoveryKey, "recovery-key", "", "Recovery key the backup was encrypted with (prompted for when omitted)")
	restoreCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Restore without confirmation")
	restoreCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	policiesCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	policiesUpdateCmd.Flags().StringVar(&policiesUpdateFile, "file", "", "JSON file with the full policies payload (use - for stdin); replaces all policies")
	policiesUpdateCmd.Flags().StringVar(&policiesUpdateID, "policy-id", "", "Policy ID to update (defaults to the sole policy)")
	policiesUpdateCmd.Flags().BoolVar(&policiesUpdateEnabled, "enabled", false, "Enable the policy")
	policiesUpdateCmd.Flags().BoolVar(&policiesUpdateDisabled, "disabled", false, "Disable the policy")
	policiesUpdateCmd.Flags().StringVar(&policiesUpdateSchedule, "schedule", "", "Cron schedule for automatic backups")
	policiesUpdateCmd.Flags().IntVar(&policiesUpdateRetention, "retention", 0, "Number of backups to retain")
	policiesUpdateCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	policiesUpdateCmd.MarkFlagsMutuallyExclusive("enabled", "disabled")

	recoveryKeyGenerateCmd.Flags().BoolVar(&generateSave, "save", false, "Store the generated recovery key on the server")
	recoveryKeyGenerateCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	recoveryKeySetCmd.Flags().StringVar(&setRecoveryKey, "recovery-key", "", "Recovery key to store (prompted for when omitted)")
	recoveryKeySetCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}
