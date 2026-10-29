// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original actual-HTTP serialized response and read-deadline boundary proofs.

package backuptarget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestVolumeResponseBudgetRefusesWholeSerializedBodyAndSetsReadDeadline(t *testing.T) {
	router, api := humatest.New(t)
	group := BudgetAPI(api)
	type output struct{ Body string }
	huma.Get(group, "/large", func(ctx context.Context, _ *struct{}) (*output, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 60, time.Until(deadline).Seconds(), 1)
		// JSON escaping expands this beyond the byte budget even though the
		// application string is smaller: enforce the serialized wire size.
		return &output{Body: strings.Repeat("\x00", dto.MaxVolumeResponseBytes/2)}, nil
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/large", nil))
	require.Equal(t, 422, response.Code)
	require.Less(t, response.Body.Len(), 1024, "no truncated data is sent")
	require.Contains(t, response.Header().Get("Content-Type"), "application/problem+json")
	var problem dto.Problem
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &problem))
	require.Equal(t, "volume.backup.listing-too-large", problem.Code)
}
func TestVolumeResponseBudgetPreservesHeadersAndExemptsDownloads(t *testing.T) {
	router, api := humatest.New(t)
	group := BudgetAPI(api)
	type output struct {
		CacheControl string `header:"Cache-Control"`
		Body         string
	}
	huma.Get(group, "/small", func(context.Context, *struct{}) (*output, error) {
		return &output{CacheControl: "no-store", Body: "ok"}, nil
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/small", nil))
	require.Equal(t, 200, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Contains(t, response.Body.String(), "ok")
	huma.Register(group, huma.Operation{OperationID: "download-volume-backup", Method: "GET", Path: "/download"}, func(context.Context, *struct{}) (*huma.StreamResponse, error) {
		return &huma.StreamResponse{Body: func(ctx huma.Context) {
			_, _ = ctx.BodyWriter().Write([]byte(strings.Repeat("x", dto.MaxVolumeResponseBytes+1)))
		}}, nil
	})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/download", nil))
	require.Equal(t, 200, response.Code)
	require.Equal(t, dto.MaxVolumeResponseBytes+1, response.Body.Len())
}
