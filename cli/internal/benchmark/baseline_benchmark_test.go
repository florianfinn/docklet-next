// Source: getarcaneapp/arcane/cli/internal/benchmark/baseline_benchmark_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package benchmark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/cli/internal/client"
	clitypes "github.com/florianfinn/docklet-next/cli/internal/types"
)

func BenchmarkClientListMediumPayload(b *testing.B) {
	payload := make(map[string]any, 0)
	items := make([]map[string]any, 0, 200)
	for i := range 200 {
		items = append(items, map[string]any{
			"id":    fmt.Sprintf("id-%d", i),
			"name":  fmt.Sprintf("name-%d", i),
			"state": "running",
		})
	}
	payload["success"] = true
	payload["data"] = items
	payload["pagination"] = map[string]any{
		"currentPage":  1,
		"itemsPerPage": 200,
		"totalItems":   200,
		"totalPages":   1,
	}
	raw, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	cfg := &clitypes.Config{ServerURL: srv.URL, APIKey: "arc_test_key"}
	c, err := client.New(cfg)
	if err != nil {
		require.FailNowf(b, "benchmark client setup failed", "new client: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, getErr := c.Get(b.Context(), "/api/environments/0/containers?limit=200")
		if getErr != nil {
			require.FailNowf(b, "benchmark request failed", "request: %v", getErr)
		}
		_ = resp.Body.Close()
	}
}
