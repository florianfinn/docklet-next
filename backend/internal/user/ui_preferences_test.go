// Source: docklet-next/issues/339
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original section, document and owner persistence tests.

package user

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/preferences"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
)

func TestPF4SectionAndDocumentLimits(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "preferences.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Exec("INSERT INTO users(id,username,password_hash,time_format) VALUES('first','first','','auto'),('second','second','','auto')").Error)
	s := NewUserService(db, nil, nil)
	for _, tc := range []struct {
		section string
		value   any
		code    string
	}{{"unknown", map[string]any{}, "preferences.unknown-section"}, {"views", []any{}, "preferences.invalid"}, {"views", nil, "preferences.invalid"}, {"views", map[string]any{"v": strings.Repeat("x", dto.SectionLimit)}, "preferences.too-large"}} {
		_, err := s.PutUIPreferences(t.Context(), "first", tc.section, tc.value)
		require.EqualError(t, err, tc.code)
	}
	value := map[string]any{"data": strings.Repeat("x", 15000)}
	for _, section := range []string{"views", "hidden", "help", "dashboard"} {
		_, err = s.PutUIPreferences(t.Context(), "first", section, value)
		require.NoError(t, err)
	}
	_, err = s.PutUIPreferences(t.Context(), "first", "fleetView", value)
	require.EqualError(t, err, "preferences.too-large")
	saved, err := s.UIPreferences(t.Context(), "first")
	require.NoError(t, err)
	require.EqualValues(t, 4, saved.Revision)
	require.Len(t, saved.Sections, 4)
	other, err := s.UIPreferences(t.Context(), "second")
	require.NoError(t, err)
	require.Empty(t, other.Sections)
	_, err = s.UIPreferences(t.Context(), "agent")
	require.EqualError(t, err, "preferences.no-user")
	require.Equal(t, emptyUIPreferences(), decodeUIPreferences(t.Context(), "malformed"))
	require.Equal(t, emptyUIPreferences(), decodeUIPreferences(t.Context(), "null"))
}
