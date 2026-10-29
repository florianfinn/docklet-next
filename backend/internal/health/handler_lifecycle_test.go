// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original health lifecycle tests.

package health

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestHealthFailsOnceLifecycleEnds(t *testing.T) {
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	_, api := humatest.New(t)
	RegisterRoutes(api, func() bool { return appCtx.Err() == nil })

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if got := api.Do(method, "/health").Code; got != http.StatusOK {
			t.Fatalf("%s /health while running = %d, want 200", method, got)
		}
	}
	cancelApp()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		resp := api.Do(method, "/health")
		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s /health after lifecycle end = %d, want 503", method, resp.Code)
		}
	}
}

func TestHealthWithoutLivenessStaysUp(t *testing.T) {
	_, api := humatest.New(t)
	RegisterRoutes(api, nil)
	if got := api.Do(http.MethodGet, "/health").Code; got != http.StatusOK {
		t.Fatalf("GET /health = %d, want 200", got)
	}
}
