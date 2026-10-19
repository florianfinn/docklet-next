// Source: docklet-next/docs/design/runtime-control.md
// Revision: 281d561206c422724c50a8fe45588fb9ca6980153eaf2aeb6afce07f8cae0e31
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original current/pending Compose namespace planning regression coverage.

package project

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

const namespaceVolumeCompose = "services:\n  app:\n    image: registry.example.com/app:1\n    volumes:\n      - data:/data\nvolumes:\n  data: {}\n"

// Only the real Docker SDK's read requests are served. Planning must never
// create, copy, remove or rename a volume; Apply and Rollback are not called.
type namespaceVolumeReads struct {
	mu                                               sync.Mutex
	names                                            []string
	writes                                           int
	attached, targetExists, sourceError, targetError bool
	source, target                                   string
	exists                                           map[string]bool
}

func namespaceVolumeDocker(t *testing.T, svc *ProjectService, reads *namespaceVolumeReads) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.mu.Lock()
		defer reads.mu.Unlock()
		if r.Method != http.MethodGet {
			reads.writes++
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, name, ok := strings.Cut(r.URL.Path, "/volumes/"); ok {
			reads.names = append(reads.names, name)
			if (name == reads.source && reads.sourceError) || (name == reads.target && reads.targetError) {
				http.Error(w, "synthetic inspect failure", 500)
				return
			}
			if !reads.exists[name] && !(name == reads.target && reads.targetExists) {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Name": name, "Driver": "local"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			if reads.attached {
				_, _ = w.Write([]byte(`[{"Id":"example-attached","Names":["/example-app"],"Mounts":[{"Type":"volume","Name":"beta_stack_data","Destination":"/data"}]}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
			return
		}
		// The path mapper may inspect its own container; the synthetic daemon has
		// no mount metadata. Do not record environment-specific container IDs.
		http.NotFound(w, r)
	}))
	t.Cleanup(func() { server.CloseClientConnections(); server.Close(); require.Zero(t, reads.writes) })
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
}

func TestComposeNameVolumePlanNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name, current, env, override                string
		pendingCompose, pendingEnv, pendingOverride *string
		source, target                              string
		sourceAbsent, wrongDefaultPresent           bool
	}{
		{name: "literal default source absent", current: "name: beta_stack\n", pendingCompose: new("name: gamma\n" + namespaceVolumeCompose), source: "beta_stack", target: "gamma"},
		{name: "literal wrong default source present", current: "name: beta_stack\n", pendingCompose: new("name: gamma\n" + namespaceVolumeCompose), source: "beta_stack", target: "gamma", wrongDefaultPresent: true},
		{name: "actual source absent wrong default present", current: "name: beta_stack\n", pendingCompose: new("name: gamma\n" + namespaceVolumeCompose), source: "beta_stack", target: "gamma", sourceAbsent: true, wrongDefaultPresent: true},
		{name: "interpolated current and submitted env", current: "name: ${STACK_NAME}\n", env: "STACK_NAME=beta_stack\n", pendingEnv: new("STACK_NAME=gamma\n"), source: "beta_stack", target: "gamma"},
		{name: "submitted override", current: "name: base_stack\n", override: "name: beta_stack\n", pendingOverride: new("name: gamma\n"), source: "beta_stack", target: "gamma"},
		{name: "removed override", current: "name: base_stack\n", override: "name: beta_stack\n", pendingOverride: new(""), source: "beta_stack", target: "base_stack"},
		{name: "unchanged effective namespace", current: "name: beta_stack\n", source: "beta_stack", target: "beta_stack"},
		{name: "legacy stored default rename", source: "alpha", target: "gamma"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", tc.current+namespaceVolumeCompose)
			if tc.env != "" {
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte(tc.env), 0600))
			}
			if tc.override != "" {
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.override.yaml"), []byte(tc.override), 0600))
			}
			// Deliberately stale observation must not become planning authority.
			row.ComposeProjectName = new("stale_observation")
			require.NoError(t, svc.db.Model(row).Update("compose_project_name", *row.ComposeProjectName).Error)
			before := *row
			storedBefore, err := svc.GetProjectFromDatabaseByID(t.Context(), row.ID)
			require.NoError(t, err)
			reads := &namespaceVolumeReads{source: tc.source + "_data", target: tc.target + "_data", exists: map[string]bool{tc.source + "_data": !tc.sourceAbsent, "alpha_data": tc.wrongDefaultPresent || tc.source == "alpha"}}
			namespaceVolumeDocker(t, svc, reads)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			migration, err := svc.prepareProjectRenameVolumeMigration(ctx, row, "gamma", root, tc.pendingCompose, tc.pendingEnv, tc.pendingOverride)
			require.NoError(t, err)
			if tc.source == tc.target || tc.sourceAbsent {
				require.Nil(t, migration)
			} else {
				require.NotNil(t, migration, "current effective source must be included even when the stored-default source is absent")
				journal, ok := migration.(volumetypes.JournalSource)
				require.True(t, ok)
				entries := journal.JournalVolumes()
				require.Len(t, entries, 1)
				require.Equal(t, "data", entries[0].Key)
				require.Equal(t, tc.source+"_data", entries[0].OldName, "journal must identify the current executable model namespace")
				require.Equal(t, tc.target+"_data", entries[0].NewName, "journal must identify the pending executable model namespace")
			}
			reads.mu.Lock()
			defer reads.mu.Unlock()
			if tc.source == tc.target {
				require.Empty(t, reads.names)
			} else {
				require.Contains(t, reads.names, tc.source+"_data")
				require.Contains(t, reads.names, tc.target+"_data")
				if tc.source != "alpha" {
					require.NotContains(t, reads.names, "alpha_data")
				}
			}
			require.Equal(t, before, *row)
			stored, err := svc.GetProjectFromDatabaseByID(ctx, row.ID)
			require.NoError(t, err)
			require.Equal(t, *storedBefore, *stored)
			bytes, err := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			require.NoError(t, err)
			require.Equal(t, tc.current+namespaceVolumeCompose, string(bytes))
			if tc.env != "" {
				bytes, err = os.ReadFile(filepath.Join(row.Path, ".env"))
				require.NoError(t, err)
				require.Equal(t, tc.env, string(bytes))
			}
			if tc.override != "" {
				bytes, err = os.ReadFile(filepath.Join(row.Path, "compose.override.yaml"))
				require.NoError(t, err)
				require.Equal(t, tc.override, string(bytes))
			}
			require.Empty(t, projectUpdatePreviewDirs(t, root))
			t.Logf("read-only volume inspections: %v", reads.names)
		})
	}
}

func TestComposeNameVolumePlanRefusalsAndExclusions(t *testing.T) {
	for _, kind := range []string{"malformed current", "malformed pending", "malformed current env", "malformed pending env", "missing current", "attached source", "target conflict", "source inspect failure", "target inspect failure", "running", "explicit volume", "external volume"} {
		t.Run(kind, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			current := "name: beta_stack\n" + namespaceVolumeCompose
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", current)
			pending := new("name: gamma\n" + namespaceVolumeCompose)
			var pendingEnv *string
			reads := &namespaceVolumeReads{source: "beta_stack_data", target: "gamma_data", exists: map[string]bool{"beta_stack_data": true, "alpha_data": true}}
			expectErr, noVolumeReads := false, false
			switch kind {
			case "malformed current":
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("services: ["), 0600))
				expectErr = true
				noVolumeReads = true
			case "malformed pending":
				pending = new("services: [")
				expectErr = true
				noVolumeReads = true
			case "malformed current env":
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("INVALID='unterminated\n"), 0600))
				expectErr = true
				noVolumeReads = true
			case "malformed pending env":
				pendingEnv = new("INVALID='unterminated\n")
				expectErr = true
				noVolumeReads = true
			case "missing current":
				require.NoError(t, os.Remove(filepath.Join(row.Path, "compose.yaml")))
				pending = nil
				noVolumeReads = true
			case "attached source":
				reads.attached = true
				expectErr = true
			case "target conflict":
				reads.targetExists = true
				expectErr = true
			case "source inspect failure":
				reads.sourceError = true
				expectErr = true
			case "target inspect failure":
				reads.targetError = true
				expectErr = true
			case "running":
				row.Status = ProjectStatusRunning
				noVolumeReads = true
			case "explicit volume":
				pending = new(strings.Replace(*pending, "data: {}", "data:\n    name: fixed-data", 1))
				noVolumeReads = true
			case "external volume":
				pending = new(strings.Replace(*pending, "data: {}", "data:\n    external: true", 1))
				noVolumeReads = true
			}
			sourceBefore, sourceErr := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			envBefore, envErr := os.ReadFile(filepath.Join(row.Path, ".env"))
			namespaceVolumeDocker(t, svc, reads)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			migration, err := svc.prepareProjectRenameVolumeMigration(ctx, row, "gamma", root, pending, pendingEnv, nil)
			if expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, migration)
			sourceAfter, afterErr := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			require.Equal(t, os.IsNotExist(sourceErr), os.IsNotExist(afterErr))
			require.Equal(t, sourceBefore, sourceAfter)
			envAfter, afterErr := os.ReadFile(filepath.Join(row.Path, ".env"))
			require.Equal(t, os.IsNotExist(envErr), os.IsNotExist(afterErr))
			require.Equal(t, envBefore, envAfter)
			reads.mu.Lock()
			defer reads.mu.Unlock()
			if noVolumeReads {
				require.Empty(t, reads.names)
			}
			require.Empty(t, projectUpdatePreviewDirs(t, root))
		})
	}
}
