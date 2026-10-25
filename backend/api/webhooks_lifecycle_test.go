// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Public webhook tokens receive typed retirement before asynchronous acceptance.

package api

import (
	"crypto/sha256"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebhookLifecycleRetirementHTTPHasNoAcceptanceEffects(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "webhook.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	svc := webhook.NewWebhookService(db, nil, nil, nil, nil, nil)
	router := echo.New()
	RegisterWebhookTrigger(router.Group("/api"), svc, handlerutil.NewActivityAppContext(t.Context()))
	raw := "arc_wh_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	row := &webhook.Webhook{Name: "example", TokenHash: fmt.Sprintf("%x", sha256.Sum256([]byte(raw))), TokenPrefix: raw[:15], Enabled: true, EnvironmentID: "0", TargetType: webhook.WebhookTargetTypeUpdater, ActionType: webhook.WebhookActionTypeRun}
	require.NoError(t, db.Create(row).Error)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/webhooks/trigger/"+raw, nil))
	require.Equal(t, 409, rec.Code)
	require.Contains(t, rec.Body.String(), "update.path-retired")
	require.NotContains(t, rec.Body.String(), raw)
	require.NoError(t, db.First(row, "id = ?", row.ID).Error)
	require.Nil(t, row.LastTriggeredAt)
}
