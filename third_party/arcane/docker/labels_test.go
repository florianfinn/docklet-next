// Source: getarcaneapp/kit/docker/labels_test.go
// Revision: e3d12f637964a6edd495f9b31a10d2f0a40fa127
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposeLabels(t *testing.T) {
	labels := map[string]string{
		ComposeProjectLabelKey: " myproj ",
		ComposeServiceLabelKey: "web",
	}
	{

		got := ComposeProjectLabel(labels)
		assert.Equal(t, "myproj", got,
			"ComposeProjectLabel() = %q, want %q", got, "myproj")
	}
	{

		got := ComposeServiceLabel(labels)
		assert.Equal(t, "web", got,
			"ComposeServiceLabel() = %q, want %q", got, "web")
	}
	{

		got := ComposeProjectLabel(nil)
		assert.Empty(t, got,
			"ComposeProjectLabel(nil) = %q, want %q", got, "")
	}
	{

		got := ComposeServiceLabel(map[string]string{})
		assert.Empty(t, got,
			"ComposeServiceLabel(empty) = %q, want %q", got, "")
	}
}
