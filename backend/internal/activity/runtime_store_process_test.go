// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Separate executable lifetimes prove store generation and compact continuity.

package activity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeStoreProcessFixture(t *testing.T) {
	mode := os.Getenv("RUNTIME_STORE_CHILD_MODE")
	if mode == "" {
		return
	}
	db, err := database.InitializeHub(t.Context(), os.Getenv("RUNTIME_STORE_CHILD_URL"), database.MigrationOptions{})
	if mode == "expect-lock" {
		require.ErrorContains(t, err, "hub.already_running")
		require.Nil(t, db)
		return
	}
	require.NoError(t, err)
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	var generation string
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		generation, err = runtimeGenerationTxInternal(tx, now)
		return err
	})
	require.NoError(t, err)
	if mode == "read" {
		require.Equal(t, os.Getenv("RUNTIME_STORE_CHILD_GENERATION"), generation)
		id := runtimeIdentityInternal{RunID: os.Getenv("RUNTIME_STORE_CHILD_RUN"), StoreGeneration: generation,
			ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0",
			ExecutorGeneration: generation, RequestDigest: "example-process-digest"}
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			row, err := runtimeLookupBindingTxInternal(tx, id, runtimeBindingInternal{Key: os.Getenv("RUNTIME_STORE_CHILD_KEY"), CallerID: "example-owner"})
			if err != nil {
				return err
			}
			require.Equal(t, id.RunID, row.RunID)
			require.Equal(t, "receipt", row.Kind)
			return nil
		}))
	}
	run, key := os.Getenv("RUNTIME_STORE_CHILD_RUN"), os.Getenv("RUNTIME_STORE_CHILD_KEY")
	if mode == "seed" {
		run, key = uuid.NewString(), uuid.NewString()
		request := runtimeAdmissionInternal{
			Identity: runtimeIdentityInternal{RunID: run, StoreGeneration: generation,
				ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0",
				ExecutorGeneration: generation, RequestDigest: "example-process-digest"},
			Binding:     runtimeBindingInternal{Key: key, CallerID: "example-owner"},
			OperationID: "deploy-project", TargetKind: "stack", TargetID: "example-project",
			GateScope: "stack:example-project", GateKey: "deploy-project:example-process-digest",
		}
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			_, err := runtimeAdmitTxInternal(tx, now, request)
			return err
		}))
	}
	// Close pools and lifetime ownership before the next process is allowed to open.
	require.NoError(t, db.Close())
	fmt.Printf("RUNTIME_STORE:%s|%s|%s\n", generation, run, key)
}

func runtimeChild(t *testing.T, url, mode string, previous []string) []string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestRuntimeStoreProcessFixture$", "-test.v")
	command.Env = append(os.Environ(), "RUNTIME_STORE_CHILD_URL="+url, "RUNTIME_STORE_CHILD_MODE="+mode)
	if previous != nil {
		command.Env = append(command.Env, "RUNTIME_STORE_CHILD_GENERATION="+previous[0], "RUNTIME_STORE_CHILD_RUN="+previous[1], "RUNTIME_STORE_CHILD_KEY="+previous[2])
	}
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	if mode == "expect-lock" {
		return nil
	}
	for _, line := range strings.Split(string(output), "\n") {
		if data, ok := strings.CutPrefix(line, "RUNTIME_STORE:"); ok {
			parts := strings.Split(data, "|")
			require.Len(t, parts, 3)
			parsed, err := uuid.Parse(parts[0])
			require.NoError(t, err)
			require.EqualValues(t, 4, parsed.Version())
			require.Equal(t, uuid.RFC4122, parsed.Variant())
			return parts
		}
	}
	t.Fatalf("child did not return store evidence: %s", output)
	return nil
}

func TestRuntimeStoreSeparateProcessContinuityAndFreshness(t *testing.T) {
	url := sqlitestore.URL(filepath.Join(t.TempDir(), "continuity.db"))
	first := runtimeChild(t, url, "seed", nil)
	second := runtimeChild(t, url, "read", first)
	require.Equal(t, first, second)
	// The parent opens only after both children have exited, then retains the lock
	// while another child proves it cannot acquire a concurrent hub lifetime.
	db, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	runtimeChild(t, url, "expect-lock", nil)
	var generation string
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		generation, err = runtimeGenerationTxInternal(tx, time.Now())
		return err
	}))
	require.Equal(t, first[0], generation)
	require.NoError(t, db.Close())
	third := runtimeChild(t, url, "read", first)
	require.Equal(t, first, third)
	fresh := runtimeChild(t, sqlitestore.URL(filepath.Join(t.TempDir(), "fresh.db")), "seed", nil)
	require.NotEqual(t, first[0], fresh[0])
}
