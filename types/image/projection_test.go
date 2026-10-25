// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package image

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestUpdateProjectionTruthAndSecrets(t *testing.T) {
	for _, tc := range []struct{ name, current, next, cd, nd, platform, kind, problem, want string }{{"equal", "1.0.0", "1.0.0", "a", "a", "linux/amd64", "digest", "", "current"}, {"major", "1.0.0", "2.0.0", "a", "b", "linux/amd64", "tag", "", "version"}, {"minor", "1.0.0", "1.1.0", "a", "b", "linux/amd64", "tag", "", "version"}, {"patch", "1.0.0", "1.0.1", "a", "b", "linux/amd64", "tag", "", "version"}, {"rebuild", "1.0.0", "1.0.0", "a", "b", "linux/amd64", "digest", "", "rebuild"}, {"index-only", "1.0.0", "1.0.0", "a", "b", "", "digest", "", "unknown"}, {"missing", "", "", "", "b", "linux/amd64", "digest", "", "unknown"}, {"local", "", "", "", "", "", "local", "", "local"}, {"private-denial", "1.0.0", "2.0.0", "a", "b", "linux/amd64", "tag", "synthetic private failure", "error"}} {
		t.Run(tc.name, func(t *testing.T) {
			v := UpdateInfo{CheckTime: time.Now(), ImageRef: "registry.example.com/app:1.0.0", CurrentVersion: tc.current, LatestVersion: tc.next, CurrentDigest: tc.cd, LatestDigest: tc.nd, Platform: tc.platform, UpdateType: tc.kind, Error: tc.problem, AuthUsername: "synthetic-credential-identity", UsedCredential: true}
			ProjectUpdate(&v)
			require.Equal(t, tc.want, v.State)
			require.Equal(t, tc.want == "version" || tc.want == "rebuild", v.HasUpdate)
			data, err := json.Marshal(v)
			require.NoError(t, err)
			require.NotContains(t, string(data), "synthetic-")
		})
	}
}
func TestPullRejectsInlinePresenceAndConflictingTag(t *testing.T) {
	for _, raw := range []string{`{"imageName":"registry.example.com/app:1.0.0","auth":null}`, `{"imageName":"registry.example.com/app:1.0.0","credentials":[]}`} {
		var options PullOptions
		require.NoError(t, json.Unmarshal([]byte(raw), &options))
		require.Equal(t, "registry.inline-credentials-not-allowed", Neutral(options.Validate(), "unexpected", 500).Problem.Code)
	}
	require.Error(t, (PullOptions{ImageName: "registry.example.com/app@sha256:" + strings.Repeat("a", 64), Tag: "1.0.0"}).Validate())
	require.Error(t, (PullOptions{ImageName: "registry.example.com/app:1.0.0", Tag: "2.0.0"}).Validate())
	require.Equal(t, "registry.example.com/app:1.0.0", (PullOptions{ImageName: "registry.example.com/app:1.0.0", Tag: "1.0.0"}).GetFullImageName())
}
func TestAgentUpgradeRunningHubCap(t *testing.T) {
	require.NoError(t, ValidateAgentUpgradeTarget("1.9.0", "2.0.0"))
	for _, tc := range []struct{ target, hub, code string }{{"", "2.0.0", "update.invalid-target"}, {"2.1.0", "2.0.0", "update.requires-hub-update"}, {"custom", "2.0.0", "update.system-version-unknown"}, {"2.0.0", "development", "update.system-version-unknown"}} {
		require.Equal(t, tc.code, Neutral(ValidateAgentUpgradeTarget(tc.target, tc.hub), "unexpected", 500).Problem.Code)
	}
}
