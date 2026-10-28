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
)

// A fatal actor-host loss exits the process; only the shipped restart policy brings it back.
func TestShippedSnippetsKeepRestartPolicy(t *testing.T) {
	directRun, directCompose := Direct("https://hub.example.com", "https://agent.example.com:3553", "key")
	edgeRun, edgeCompose := Edge("https://hub.example.com", "key")
	for name, snippet := range map[string]string{"direct run": directRun, "edge run": edgeRun} {
		if !strings.Contains(snippet, "--restart unless-stopped") {
			t.Errorf("%s snippet lacks --restart unless-stopped", name)
		}
	}
	for name, snippet := range map[string]string{"direct compose": directCompose, "edge compose": edgeCompose} {
		if !strings.Contains(snippet, "restart: unless-stopped") {
			t.Errorf("%s snippet lacks restart: unless-stopped", name)
		}
	}
	hubCompose, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "deploy", "hub", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hubCompose), "restart: unless-stopped") {
		t.Error("deploy/hub/compose.yaml lacks restart: unless-stopped")
	}
}
