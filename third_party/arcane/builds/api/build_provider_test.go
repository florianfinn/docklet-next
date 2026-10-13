// Source: getarcaneapp/kit/builds/api/build_provider_test.go
// Revision: 83d91f19f60b2967cc85c46161c98203e80e4989
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectDepotArchInternal(t *testing.T) {
	assert.Equal(t, "arm64", selectDepotArchInternal([]string{"linux/arm64/v8"}))
	assert.Equal(t, "amd64", selectDepotArchInternal([]string{"linux/x86_64"}))
	assert.Equal(t, "arm64", selectDepotArchInternal([]string{"not a platform", "linux/aarch64"}))
}
