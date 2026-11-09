// Source: docklet-next/issues/409
// Revision: 3cfaa8ee183870518a40ab1f3cd75e69a4c778c8
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original details merge of a configured tag-policy record with the live container check.

package project

import (
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/updater/labels"
)

// Configured records and container checks both carry the selected version, so
// the details merge keeps one candidate when they agree and clears it otherwise.
func TestConfiguredTagPolicyRecordMergesWithTheRuntimeCheck(t *testing.T) {
	oldDigest, newDigest, otherDigest := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("c", 64)
	for _, tc := range patternTagCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			ref := "registry.example.com/app:" + tc.CurrentTag
			serviceLabels := map[string]string{labels.LabelUpdateStrategy: "tag"}
			if tc.Pattern != "" {
				serviceLabels[labels.LabelUpdateTagPattern] = tc.Pattern
			}
			at := time.Now().UTC()
			record := imageupdate.ImageUpdateRecord{ProjectID: "stack", ServiceName: "app", PolicyKey: imageref.UpdatePolicyKey(ref, serviceLabels), Repository: "registry.example.com/app", Tag: tc.CurrentTag, CheckTime: at, CurrentVersion: tc.Update.CurrentVersion, LatestVersion: new(tc.Update.LatestVersion), CurrentDigest: new(oldDigest), LatestDigest: new(newDigest), Platform: new("native-v1:classic:linux/amd64"), UpdateType: "tag", HasUpdate: true}
			require.Equal(t, tc.Update.State, record.UpdateInfo().State)
			services := []types.ServiceConfig{{Name: "app", Image: ref, Labels: serviceLabels}}
			merge := func(latestVersion, latestDigest string) *imagetypes.UpdateInfo {
				runtime := &imagetypes.UpdateInfo{ImageRef: ref, CheckTime: at.Add(time.Second), CurrentVersion: tc.Update.CurrentVersion, LatestVersion: latestVersion, CurrentDigest: oldDigest, LatestDigest: latestDigest, Platform: "linux/amd64", UpdateType: "tag"}
				imagetypes.ProjectUpdate(runtime)
				return BuildConfiguredUpdateInfo("stack", services, nil, []imageupdate.ImageUpdateRecord{record}, map[string]*imagetypes.UpdateInfo{"app": runtime}).ServiceUpdates["app"].UpdateInfo
			}

			agreed := merge(tc.Update.LatestVersion, newDigest)
			require.NotNil(t, agreed)
			require.Equal(t, "version", agreed.State)
			require.Equal(t, tc.Update.LatestVersion, agreed.LatestVersion)
			require.Equal(t, newDigest, agreed.LatestDigest)

			disagreed := merge("1.2.0", otherDigest)
			require.NotNil(t, disagreed)
			require.Empty(t, disagreed.LatestVersion, "different candidates are not presented as one")
			require.Empty(t, disagreed.LatestDigest)
		})
	}
}
