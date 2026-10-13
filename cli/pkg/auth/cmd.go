// Source: getarcaneapp/arcane/cli/pkg/auth/cmd.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.

package auth

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/spf13/cobra"

	"github.com/florianfinn/docklet-next/cli/internal/cmdutil"
	"github.com/florianfinn/docklet-next/cli/internal/config"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/types"
)

var jsonOutput bool

// AuthCmd is the parent command for authentication operations
var AuthCmd = &cobra.Command{
	Use:     "auth",
	Aliases: []string{"authentication"},
	Short:   "Authentication operations",
}

var loginCmd = &cobra.Command{
	Use:          "login",
	Short:        "Login with a local account",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("username")
		if strings.TrimSpace(username) == "" {
			return errors.New("username is required")
		}
		password, err := output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Password", "password")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		c, err := cmdutil.UnauthClientFromCommand(cmd)
		if err != nil {
			return err
		}
		result, err := c.PostJSON[auth.AuthenticationResponse](cmd.Context(), types.AuthLogin(), auth.Login{Username: username, Password: password})
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		if !result.Data.Success || result.Data.Token == "" {
			return errors.New("login failed: no access token returned")
		}
		if cmdutil.JSONOutputEnabled(cmd) || jsonOutput {
			return cmdutil.PrintJSON(result.Data)
		}
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.JWTToken = result.Data.Token
		cfg.RefreshToken = result.Data.RefreshToken
		cfg.APIKey = ""
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
		output.Success("Login successful")
		path, _ := config.ConfigPath()
		output.KeyValue("JWT token saved to config", path)
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:          "logout",
	Short:        "Logout from Arcane",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.PostJSON[base.MessageResponse](cmd.Context(), types.AuthLogout(), nil)
		if err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}

		// Clear token from config after successful API logout.
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.JWTToken = ""
		cfg.RefreshToken = ""
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("failed to clear token: %w", saveErr)
		}

		if cmdutil.JSONOutputEnabled(cmd) || jsonOutput {
			return cmdutil.PrintJSON(result.Data)
		}

		output.Success("Logout successful")
		path, _ := config.ConfigPath()
		output.KeyValue("JWT token cleared from config", path)
		return nil
	},
}

var meCmd = &cobra.Command{
	Use:          "me",
	Short:        "Get current user information",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		result, err := c.GetJSON[user.User](cmd.Context(), types.AuthMe())
		if err != nil {
			return fmt.Errorf("failed to get user info: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) || jsonOutput {
			return cmdutil.PrintJSON(result.Data)
		}

		output.Header("Current User")
		return cmdutil.PrintJSON(result.Data)
	},
}

var passwordCmd = &cobra.Command{
	Use:          "password",
	Short:        "Change password",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentPassword, _ := cmd.Flags().GetString("current")
		newPassword, _ := cmd.Flags().GetString("new")

		if currentPassword == "" {
			var err error
			if currentPassword, err = output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Current password", "current"); err != nil {
				return fmt.Errorf("failed to read current password: %w", err)
			}
		}

		if newPassword == "" {
			var err error
			if newPassword, err = output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "New password", "new"); err != nil {
				return fmt.Errorf("failed to read new password: %w", err)
			}
		}

		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		changeReq := auth.PasswordChange{
			CurrentPassword: currentPassword,
			NewPassword:     newPassword,
		}

		reqBody, err := json.Marshal(changeReq)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		result, err := c.PostJSON[base.MessageResponse](cmd.Context(), types.AuthPassword(), reqBody)
		if err != nil {
			return fmt.Errorf("password change failed: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) || jsonOutput {
			return cmdutil.PrintJSON(result.Data)
		}

		output.Success("Password changed successfully")
		return nil
	},
}

var refreshCmd = &cobra.Command{
	Use:          "refresh",
	Short:        "Refresh authentication token",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		refreshToken, _ := cmd.Flags().GetString("refresh-token")

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if refreshToken == "" {
			refreshToken = cfg.RefreshToken
		}
		if refreshToken == "" {
			if refreshToken, err = output.Secret(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "Refresh token", "refresh-token"); err != nil {
				return fmt.Errorf("failed to read refresh token: %w", err)
			}
		}

		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		reqBody, err := json.Marshal(map[string]string{"refreshToken": refreshToken})
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		result, err := c.PostJSON[auth.TokenRefreshResponse](cmd.Context(), types.AuthRefresh(), reqBody)
		if err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) || jsonOutput {
			return cmdutil.PrintJSON(map[string]any{
				"token":        result.Data.Token,
				"refreshToken": result.Data.RefreshToken,
				"expiresAt":    result.Data.ExpiresAt,
			})
		}

		// Save new JWT token to config
		cfg.JWTToken = result.Data.Token
		cfg.APIKey = ""
		if result.Data.RefreshToken != "" {
			cfg.RefreshToken = result.Data.RefreshToken
		}
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("failed to save token: %w", saveErr)
		}

		output.Success("Token refreshed successfully")
		path, _ := config.ConfigPath()
		output.KeyValue("New JWT token saved to config", path)
		return nil
	},
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
	AuthCmd.AddCommand(meCmd)
	AuthCmd.AddCommand(passwordCmd)
	AuthCmd.AddCommand(refreshCmd)

	// Login command flags
	loginCmd.Flags().String("username", "", "Local username")
	loginCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	// Password command flags
	passwordCmd.Flags().String("current", "", "Current password")
	passwordCmd.Flags().String("new", "", "New password")
	passwordCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	// Refresh command flags
	refreshCmd.Flags().String("refresh-token", "", "Refresh token")
	refreshCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	// Global JSON output flags
	logoutCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	meCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}
