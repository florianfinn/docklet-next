// Source: docklet-next/docs/design/identity.md
// Revision: 27b31b131099aa1036d237769d18a2d03861d01c
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original executable time-zone packaging regression test.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

func TestEmbeddedTimeZones(t *testing.T) {
	if os.Getenv("DOCKLET_TZ_TEST_CHILD") == "1" {
		cfg := &config.Config{Timezone: os.Getenv("TZ")}
		if got := cfg.GetLocation().String(); got != "Europe/Berlin" {
			t.Fatalf("effective time zone = %q, want Europe/Berlin", got)
		}
		return
	}

	// ZONEINFO alone still allows OS fallback. Check the production dependency
	// graph too, so a host's installed database cannot mask a missing embed.
	for _, tags := range []string{"", "exclude_frontend"} {
		cmd := exec.Command("go", "list", "-deps", "-tags="+tags, "-f", "{{.ImportPath}}", ".")
		deps, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("list executable dependencies: %v\n%s", err, deps)
		}
		if !strings.Contains("\n"+string(deps), "\ntime/tzdata\n") {
			t.Fatalf("executable tags %q does not embed the time-zone database", tags)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	emptyRoot := t.TempDir()
	cmd := exec.Command(executable, "-test.run=^TestEmbeddedTimeZones$")
	cmd.Env = append(os.Environ(), "DOCKLET_TZ_TEST_CHILD=1", "TZ=Europe/Berlin",
		"ZONEINFO="+filepath.Join(emptyRoot, "missing-zoneinfo"), "GOROOT="+emptyRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("time-zone subprocess: %v\n%s", err, output)
	}
}
