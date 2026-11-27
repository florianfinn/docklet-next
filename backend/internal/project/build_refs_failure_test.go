// Source: docklet-next/issues/674
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package project

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLoadFailureKeepsBuildReferences(t *testing.T) {
	db := setupProjectTestDB(t)
	settings, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	refs := `["example.com/local:1"]`
	p := &Project{ID: "build-refs", Name: "example", Path: t.TempDir(), BuildImageRefsJSON: &refs}
	require.NoError(t, db.Create(p).Error)
	svc := NewProjectService(db, settings, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	svc.refreshProjectImageRefs(t.Context(), p)
	require.NotNil(t, p.BuildImageRefsJSON)
	require.Equal(t, refs, *p.BuildImageRefsJSON)
	var saved Project
	require.NoError(t, db.First(&saved, "id = ?", p.ID).Error)
	require.NotNil(t, saved.BuildImageRefsJSON)
	require.Equal(t, refs, *saved.BuildImageRefsJSON)
}
