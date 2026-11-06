// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStackImportMirrorValidation(t *testing.T) {
	for _, value := range []string{"", "https://raw.githubusercontent.com", "https://mirror.example.com/prefix", "http://localhost:1234/", "http://127.0.0.1:1234", "http://[::1]:1234"} {
		t.Run(value, func(t *testing.T) { _, err := StackImportBaseURL(value); require.NoError(t, err) })
	}
	for _, value := range []string{"http://mirror.example.com", "http://192.0.2.1", "ftp://mirror.example.com", "//mirror.example.com", "https://user:example@mirror.example.com", "https://mirror.example.com?token=example", "https://mirror.example.com?", "https://mirror.example.com/#fragment", "https://mirror.example.com:0", "https://mirror.example.com:65536", "https://mirror.example.com/../raw", "https://mirror.example.com/%2e%2e/raw", "https://mirror.example.com/a%5cb"} {
		t.Run(value, func(t *testing.T) {
			_, err := StackImportBaseURL(value)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
			require.Error(t, (&Config{StackImportGitHubRawURL: value}).ValidateStackImport())
		})
	}
}
func TestStackImportMirrorEnvironment(t *testing.T) {
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL", "")
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL_FILE", "")
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL__FILE", "")
	require.Equal(t, DefaultStackImportGitHubRawURL, Load().StackImportGitHubRawURL)
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL", "https://mirror.example.com/raw")
	require.Equal(t, "https://mirror.example.com/raw", Load().StackImportGitHubRawURL)
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL", "")
	file := filepath.Join(t.TempDir(), "mirror")
	require.NoError(t, os.WriteFile(file, []byte("https://mirror.example.com/from-file"), 0600))
	t.Setenv("STACK_IMPORT_GITHUB_RAW_URL_FILE", file)
	require.Equal(t, "https://mirror.example.com/from-file", Load().StackImportGitHubRawURL)
}
