// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAgentHubV7ImageSafetyFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v9.json")
	require.NoError(t, err)
	sum := sha256.Sum256(body)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(sum[:]))
	var fixture struct {
		Version     int
		Minimum     int `json:"minimumAgentContractVersion"`
		ImageSafety struct{ RegistryTestFields, RegistryPublicFields, UpdateInfoFields, PruneFields, ProtectionFields, ErrorCodes, InactiveCapabilities, Capabilities, UpdateStates []string }
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	require.Equal(t, 9, AgentHubVersion)
	require.Equal(t, 9, MinimumAgentVersion)
	require.Equal(t, AgentHubVersion, fixture.Version)
	require.Equal(t, MinimumAgentVersion, fixture.Minimum)
	require.False(t, AgentCompatible(6, "be97c8565af13b50db70f2447a1aa47cddb38fff918ddf03c04a3670aa046e25"))
	require.True(t, AgentCompatible(9, AgentHubFingerprint))
	require.False(t, AgentCompatible(7, "mixed-intermediate-build"))
	// Every fixture field list is checked against the real wire producer type,
	// including optional credential/config fields populated with synthetic data.
	description := "Public description"
	for _, entry := range []struct {
		value  any
		fields []string
	}{{containerregistry.RegistryTestResult{Code: "registry.authentication-failed"}, fixture.ImageSafety.RegistryTestFields}, {containerregistry.ContainerRegistry{Description: &description, AWSRegion: "example-region"}, fixture.ImageSafety.RegistryPublicFields}, {imageDTO.UpdateInfo{Platform: "linux/amd64", Code: "update.check-unavailable", AuthMethod: "anonymous", AuthRegistry: "registry.example.com", UsedCredential: true}, fixture.ImageSafety.UpdateInfoFields}, {imageDTO.PruneReport{ImagesDeleted: []string{}, Skipped: []imageDTO.PruneItem{}, Failed: []imageDTO.PruneItem{}}, fixture.ImageSafety.PruneFields}, {imageDTO.Protection{Reason: "restore-point", OwnerID: "point"}, fixture.ImageSafety.ProtectionFields}} {
		encoded, e := json.Marshal(entry.value)
		require.NoError(t, e)
		var actual map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &actual))
		want := []string{}
		for _, field := range entry.fields {
			want = append(want, strings.TrimSuffix(field, "?"))
		}
		got := []string{}
		for field := range actual {
			got = append(got, field)
		}
		sort.Strings(want)
		sort.Strings(got)
		require.Equal(t, want, got)
	}
	field, _ := reflect.TypeFor[imageDTO.Problem]().FieldByName("Code")
	require.Equal(t, fixture.ImageSafety.ErrorCodes, strings.Split(field.Tag.Get("enum"), ","))
	require.NotContains(t, fixture.ImageSafety.InactiveCapabilities, "restore")
	require.NotContains(t, fixture.ImageSafety.InactiveCapabilities, "auto-apply")
	for _, capability := range []string{"auto-apply", "policies", "hub-image-schedules", "candidate-notifications", "system-image-hints"} {
		require.Contains(t, fixture.ImageSafety.Capabilities, capability)
	}
}
