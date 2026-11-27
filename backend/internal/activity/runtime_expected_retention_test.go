// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestExpectedRetainedIntentSurvivesReopen(t *testing.T) {
	f := b1Fixture(t)
	c := imageCommand(f.command(), RuntimeUpdateProject)
	run := uuid.NewString()
	expected := []dto.ExpectedCandidate{{Service: "app", Manifest: "sha256:" + strings.Repeat("e", 64)}}
	c.Options.Lifecycle.Policy = &dto.PolicyBinding{PolicyID: "policy", Revision: 1, ChildID: run, JobRunID: "window", Expected: expected}
	require.NoError(t, runtimeSaveImageIntent(f.store.svc.db.DB, run, c))
	require.NoError(t, f.store.svc.db.Close())
	db, e := database.InitializeHub(t.Context(), sqlitestore.URL(f.store.path), database.MigrationOptions{})
	require.NoError(t, e)
	defer db.Close()
	reopened := NewRuntimeService(NewActivityService(db, nil), f.resolver)
	expected[0].Manifest = "sha256:" + strings.Repeat("b", 64)
	retained, e := reopened.retainedImageIntent(t.Context(), run, c)
	require.NoError(t, e)
	require.Len(t, retained.Options.Lifecycle.Policy.Expected, 1)
	require.Equal(t, "sha256:"+strings.Repeat("e", 64), retained.Options.Lifecycle.Policy.Expected[0].Manifest)
	var rows int64
	require.NoError(t, db.Table("runtime_image_intents").Count(&rows).Error)
	require.EqualValues(t, 1, rows)
	require.Zero(t, f.resolver.calls.Load())
}
