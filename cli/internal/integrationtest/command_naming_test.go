// Source: getarcaneapp/arcane/cli/internal/integrationtest/command_naming_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package integrationtest

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	clipkg "github.com/florianfinn/docklet-next/cli/pkg"
)

// Multi-word actions use flags or nested commands, except the explicit
// self-update, auto-update, and api-keys names. Hidden commands are exempt.
func TestNoHyphenatedCommandNames(t *testing.T) {
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		for _, child := range cmd.Commands() {
			if child.Hidden || child.Name() == "self-update" || child.Name() == "auto-update" || child.Name() == "api-keys" {
				continue
			}
			childPath := path + " " + child.Name()
			if strings.Contains(child.Name(), "-") {
				t.Errorf("command %q has a hyphenated name; use a flag or nested subcommand instead", childPath)
			}
			walk(child, childPath)
		}
	}
	root := clipkg.RootCommand()
	walk(root, root.Name())
	for _, path := range [][]string{
		{"images", "vulnerabilities", "scan"},
		{"admin", "backups", "list"},
		{"volumes", "backups", "list"},
		{"jobs", "schedules", "get"},
		{"jobs", "schedules", "update"},
		{"jobs", "list"},
		{"jobs", "run"},
		{"containers", "auto-update"},
		{"auth", "api-keys", "list"},
		{"admin", "api-keys", "list"},
	} {
		cmd, remaining, err := root.Find(path)
		require.NoError(t, err)
		require.Empty(t, remaining, "command %s was not fully resolved", strings.Join(path, " "))
		require.Equal(t, root.Name()+" "+strings.Join(path, " "), cmd.CommandPath())
	}
	for _, path := range [][]string{
		{"backups"},
		{"vulnerabilities"},
		{"jobs", "get"},
		{"jobs", "update"},
		{"containers", "autoupdate"},
		{"auth", "keys"},
		{"admin", "keys"},
	} {
		cmd, remaining, err := root.Find(path)
		require.True(t, err != nil || len(remaining) > 0 || cmd.CommandPath() != root.Name()+" "+strings.Join(path, " "),
			"old command %s is still registered", strings.Join(path, " "))
	}
}
