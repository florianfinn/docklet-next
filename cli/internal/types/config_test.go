// Source: getarcaneapp/arcane/cli/internal/types/config_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigLimitForRepos(t *testing.T) {
	cfg := &Config{}
	cfg.SetResourceLimit("repos", 42)

	for _, resource := range []string{"repos", "repo", "git-repositories", "git-repos"} {
		{
			got := cfg.LimitFor(resource)
			require.Equal(t, 42, got,
				"LimitFor(%q) = %d, want 42", resource, got)
		}
	}
}

func TestNormalizePaginatedResourceGitOpsSyncAliases(t *testing.T) {
	for _, resource := range []string{
		"gitops-syncs",
		"gitopssyncs",
		"gitops syncs",
		"gitops",
		"gitopssync",
	} {
		{
			got := NormalizePaginatedResource(resource)
			require.Equal(t, "gitops-syncs", got,
				"NormalizePaginatedResource(%q) = %q, want %q", resource, got, "gitops-syncs")
		}
	}
}
