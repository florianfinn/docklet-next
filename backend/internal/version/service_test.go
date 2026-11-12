// Source: getarcaneapp/arcane/backend/internal/version/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; digest and next-channel cases removed with their code; product release feed cases (#235).

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionService_GetAppVersionInfoDisplaysSemverPrerelease(t *testing.T) {
	svc := NewVersionService(nil, true, "", "1.0.0-rc.2", "2c3e44a10ddda540d7e19fc2a876c931fc33a426", nil, nil)

	info := svc.GetAppVersionInfo(t.Context())

	require.NotNil(t, info)
	assert.Equal(t, "v1.0.0-rc.2", info.DisplayVersion)
	assert.True(t, info.IsSemverVersion)
}

func TestVersionService_GetAppVersionInfoPreservesDevVersionInternal(t *testing.T) {
	svc := NewVersionService(nil, true, "", "dev", "unknown", nil, nil)

	info := svc.GetAppVersionInfo(t.Context())

	require.NotNil(t, info)
	assert.Equal(t, "dev", info.CurrentVersion)
	assert.Equal(t, "dev", info.DisplayVersion)
	assert.False(t, info.IsSemverVersion)
}

func TestVersionService_GetAppVersionInfoSuppressesOlderStableForPrereleaseInternal(t *testing.T) {
	feed := newRecordingFeedInternal(t, `[`+feedEntryJSON("v2.7.0")+`]`)
	// An rc build stays on the stable track; an older stable release is no target.
	svc := NewVersionService(feed.client, false, "", "2.8.0-rc.1", "revision", nil, nil)

	info := svc.GetAppVersionInfo(t.Context())

	require.NotNil(t, info)
	assert.False(t, info.UpdateAvailable)
	assert.Empty(t, info.NewestVersion)
	assert.Empty(t, info.ReleaseNotes)
	assert.Empty(t, info.ReleaseURL)
	assert.Empty(t, info.ReleasedAt)
}
