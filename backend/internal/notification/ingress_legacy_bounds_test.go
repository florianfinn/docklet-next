// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original legacy report bound regressions.

package notification

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func longPruneReportInternal() dto.DispatchRequest {
	images := make([]string, 300)
	for i := range images {
		images[i] = fmt.Sprintf("sha256:%064d", i)
	}
	dockerError := "Error response from daemon: conflict: unable to delete image (must be forced) - image is being used by stopped container\n" + strings.Repeat("ü", 80)
	return dto.DispatchRequest{Kind: dto.DispatchKindPruneReport, PruneReport: &dto.DispatchPruneReport{Result: system.PruneAllResult{ImagesDeleted: images, Errors: []string{dockerError}}}}
}

func TestLegacyPruneReportWithLongDockerTextAndManyImagesIsClampedAndDelivered(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
	token, key := "example", "key"
	require.NoError(t, db.Create(&environment.Environment{ID: "remote", Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}).Error)
	cfg := huma.DefaultConfig("test", "1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
	_, api := humatest.New(t, cfg)
	RegisterNotifications(api, s, &config.Config{})
	b, err := json.Marshal(longPruneReportInternal())
	require.NoError(t, err)
	r := api.Post("/notifications/dispatch", "X-API-Key: "+token, "Content-Type: application/json", bytes.NewReader(b))
	require.Equal(t, 200, r.Code, r.Body.String())

	clamped := longPruneReportInternal()
	require.NoError(t, validateDispatch(&clamped))
	result := clamped.PruneReport.Result
	require.Len(t, result.ImagesDeleted, 256)
	require.Len(t, result.Errors, 1)
	require.LessOrEqual(t, len(result.Errors[0]), 128)
	require.True(t, strings.HasPrefix(result.Errors[0], "Error response from daemon"))
	require.False(t, strings.ContainsFunc(result.Errors[0], unicode.IsControl))
	require.True(t, strings.ToValidUTF8(result.Errors[0], "") == result.Errors[0], "truncation keeps whole runes")
}
