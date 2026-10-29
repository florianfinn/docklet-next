// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original test pinning the restart policy that recovers an agent after a fatal exit.

package snippets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.getarcane.app/sys/crypto"
	"go.yaml.in/yaml/v4"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
)

// A fatal actor-host loss exits the process; only the shipped restart policy brings it back.
func TestShippedSnippetsKeepRestartPolicy(t *testing.T) {
	directRun, directCompose := Direct("https://hub.example.com", "https://agent.example.com:3553", "key")
	edgeRun, edgeCompose := Edge("https://hub.example.com", "key")
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "test-encryption-key-for-snippets-32bytes-min"})
	assets, mtlsRun, mtlsCompose := New(nil).GenerateMTLS(t.Context(), &edge.Config{
		EdgeMTLSMode:      edge.EdgeMTLSModeRequired,
		EdgeMTLSAssetsDir: t.TempDir(),
		AppURL:            "https://hub.example.com",
	}, "env-1", "Example", "https://hub.example.com", "key")
	if assets == nil {
		t.Fatal("mTLS snippets were not generated")
	}
	for name, snippet := range map[string]string{"direct run": directRun, "edge run": edgeRun, "mtls run": mtlsRun} {
		if !hasRunFlag(snippet, "--restart unless-stopped") {
			t.Errorf("%s snippet lacks --restart unless-stopped", name)
		}
	}
	for name, snippet := range map[string]string{"direct compose": directCompose, "edge compose": edgeCompose, "mtls compose": mtlsCompose} {
		assertComposeRestart(t, name, []byte(snippet), "")
	}
	hubCompose, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "deploy", "hub", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	assertComposeRestart(t, "deploy/hub/compose.yaml", hubCompose, "hub")
}

func hasRunFlag(snippet, flag string) bool {
	for _, line := range strings.Split(snippet, "\n") {
		if strings.TrimSuffix(strings.TrimSpace(line), " \\") == flag {
			return true
		}
	}
	return false
}

// assertComposeRestart decodes the compose file, so commented or misplaced keys do not count.
func assertComposeRestart(t *testing.T, name string, content []byte, service string) {
	t.Helper()
	var compose struct {
		Services map[string]struct {
			Restart string `yaml:"restart"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &compose); err != nil {
		t.Fatalf("%s: decode compose: %v", name, err)
	}
	if len(compose.Services) == 0 {
		t.Fatalf("%s: no services", name)
	}
	for serviceName, definition := range compose.Services {
		if service != "" && serviceName != service {
			continue
		}
		if definition.Restart != "unless-stopped" {
			t.Errorf("%s: service %s restart = %q, want unless-stopped", name, serviceName, definition.Restart)
		}
	}
	if _, ok := compose.Services[service]; service != "" && !ok {
		t.Errorf("%s: service %s missing", name, service)
	}
}
