// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package imageupdate_test

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/testfixtures"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUncheckedHandlerReads(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "reads.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	svc := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: "sha256:" + strings.Repeat("d", 64), Repository: "registry.example.com/app", Tag: "stable"}).Error)
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "test"))
	imageupdate.RegisterImageUpdates(api, nil, svc.GetUpdateInfoByImageRefs, handlerutil.NewActivityAppContext(t.Context()))
	request := httptest.NewRequest(http.MethodGet, "/environments/0/image-updates/by-refs?imageRefs=registry.example.com/app:stable,registry.example.com/absent:stable", http.NoBody)
	request = request.WithContext(context.WithValue(request.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, 200, response.Code, response.Body.String())
	var out struct {
		Data map[string]*imageDTO.UpdateInfo
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
	require.NotContains(t, out.Data, "registry.example.com/absent:stable")
	require.Equal(t, "unchecked", out.Data["registry.example.com/app:stable"].State)
	testfixtures.Read(t, "unchecked", out.Data)
}
