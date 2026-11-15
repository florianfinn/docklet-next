// Source: getarcaneapp/arcane/types/features/features_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Swarm; optional-module foundation (#472).

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
			want: features.Definition{ID: features.VulnerabilityManagement, Kind: features.Feature, SettingKey: "featureVulnerabilityManagementEnabled", DefaultEnabled: true},
		},
		{
			name: "returned definitions are independent", id: features.VulnerabilityManagement, mutateCopy: true, wantFound: true,
			want: features.Definition{ID: features.VulnerabilityManagement, Kind: features.Feature, SettingKey: "featureVulnerabilityManagementEnabled", DefaultEnabled: true},
		},
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

func TestModuleRegistryAndDecodedPaths(t *testing.T) {
	modules := features.Modules()
	if len(modules) != 1 || modules[0].ID != features.Proxy || modules[0].Kind != features.Module || modules[0].DefaultEnabled {
		t.Fatalf("unexpected optional module definitions: %+v", modules)
	}
	for _, path := range []string{"/api/modules/proxy", "/api/modules/proxy/operations", "/api/environments/example/modules/proxy/data", "/api/environments/0/modules/proxy?extra"} {
		d, ok := features.ForPath(path)
		if !ok || d.ID != features.Proxy {
			t.Fatalf("module ownership missing for %q", path)
		}
	}
	for _, path := range []string{"/api/modules/proxy-extra", "/api/environments/example/modules/proxy-extra/data", "/api/modules", "/api/environments//modules/proxy"} {
		if _, ok := features.ForPath(path); ok {
			t.Fatalf("unexpected module ownership for %q", path)
		}
	}
	modules[0].DefaultEnabled = true
	fresh := features.Modules()
	if fresh[0].DefaultEnabled {
		t.Fatal("module registry is mutable through its copy")
	}
}
