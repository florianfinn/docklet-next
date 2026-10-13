// Source: getarcaneapp/arcane/types/features/features_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package features_test

import (
	"testing"

	"github.com/florianfinn/docklet-next/types/features"
)

func TestRegistry(t *testing.T) {
	tests := []struct {
		name       string
		id         features.ID
		mutateCopy bool
		wantFound  bool
		want       features.Definition
	}{
		{
			name: "known feature", id: features.VulnerabilityManagement, wantFound: true,
			want: features.Definition{ID: features.VulnerabilityManagement, SettingKey: "featureVulnerabilityManagementEnabled", DefaultEnabled: true},
		},
		{
			name: "returned definitions are independent", id: features.VulnerabilityManagement, mutateCopy: true, wantFound: true,
			want: features.Definition{ID: features.VulnerabilityManagement, SettingKey: "featureVulnerabilityManagementEnabled", DefaultEnabled: true},
		},
		{name: "swarm defaults off", id: features.Swarm, wantFound: true, want: features.Definition{ID: features.Swarm, SettingKey: "featureSwarmEnabled", DefaultEnabled: false}},
		{name: "unknown feature", id: features.ID("unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mutateCopy {
				all := features.All()
				if len(all) == 0 {
					t.Fatal("registry is empty")
				}
				all[0].DefaultEnabled = false
			}
			definition, found := features.Lookup(tt.id)
			if found != tt.wantFound || definition != tt.want {
				t.Fatalf("Lookup(%q) = %+v, %t; want %+v, %t", tt.id, definition, found, tt.want, tt.wantFound)
			}
		})
	}
}
