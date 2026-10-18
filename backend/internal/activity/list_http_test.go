// Source: docklet-next/issues/108
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity list source-completeness contract and regression evidence.

package activity

import (
	"context"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestActivityListCompleteLocalAndDirect(t *testing.T) {
	for _, local := range []bool{false, true} {
		for _, populated := range []bool{false, true} {
			t.Run(map[bool]string{true: "local", false: "direct"}[local]+"/"+map[bool]string{true: "nonempty", false: "empty"}[populated], func(t *testing.T) {
				calls := 0
				server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) {
					calls++
					rows := []activity.Activity{}
					if populated {
						rows = append(rows, remoteActivityListRow())
					}
					writeActivityAgentPage(t, w, rows, int64(len(rows)))
				})
				f := newActivityListFixture(t, false, server.URL, server.Client())
				host := activityListHost
				want := []activity.Activity{}
				if local {
					host = "0"
					if populated {
						want = append(want, f.seed(t, host))
					}
				} else if populated {
					row := remoteActivityListRow()
					applyActivitySourceInternal(&row, host, "Example host")
					want = append(want, row)
				}
				page := assertActivityListPage(t, f.request(t, host, ""), "", want)
				require.Equal(t, int64(len(want)), page.Pagination.TotalItems)
				require.Equal(t, map[bool]int{true: 0, false: 1}[local], calls)
			})
		}
	}
}

func TestActivityListDirectHTTPFailures(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 500, 502, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Error(w, "example upstream failure", status)
			})
			f := newActivityListFixture(t, false, server.URL, server.Client())
			f.seed(t, activityListHost)
			rec := f.request(t, activityListHost, "")
			want := status
			if status == 401 {
				want = 502
			}
			require.Equal(t, want, rec.Code, rec.Body.String())
			require.Empty(t, rec.Header().Get(activityListReason))
			require.NotContains(t, rec.Body.String(), `"success":true`)
			require.Equal(t, 1, calls)
		})
	}
}

func TestActivityListExceptionalErrorsAreNotOutages(t *testing.T) {
	// These exceptional errors are injected at the proxy seam, not real transport evidence.
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"credential", remenv.ErrAgentInvalidToken, 500},
		{"cancelled transport", &remenv.TransportError{Err: context.Canceled}, 502},
		{"cancelled operation", context.Canceled, 500},
		{"unavailable text only", errors.New("edge agent is not connected"), 500},
		{"mapping", &remenv.DecodeError{Err: errors.New("example invalid data")}, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newActivityListFixture(t, false, "https://agent.example.com", nil)
			f.seed(t, activityListHost)
			calls := 0
			f.handler.environment.ProxyJSONRequest = func(context.Context, string, string, string, []byte, any) error { calls++; return tc.err }
			rec := f.request(t, activityListHost, "")
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Empty(t, rec.Header().Get(activityListReason))
			require.Equal(t, 1, calls)
		})
	}
}

func TestActivityListInvalidDataAndPagination(t *testing.T) {
	for _, tc := range []struct {
		name, body, query string
		status, calls     int
	}{
		{"malformed JSON", `{`, "", 500, 1},
		{"wrong data shape", `{"success":true,"data":"invalid"}`, "", 500, 1},
		{"wrong pagination shape", `{"success":true,"data":[],"pagination":{"totalItems":"invalid"}}`, "", 500, 1},
		{"invalid start", `{}`, "?start=invalid", 422, 0},
		{"invalid limit", `{}`, "?limit=invalid", 422, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(tc.body)) })
			f := newActivityListFixture(t, false, server.URL, server.Client())
			f.seed(t, activityListHost)
			rec := f.request(t, activityListHost, tc.query)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Empty(t, rec.Header().Get(activityListReason))
			require.Equal(t, tc.calls, calls)
		})
	}
}

func TestActivityListHubReadFailureIsNotPartialSuccess(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	f.handler.environment.ProxyJSONRequest = func(context.Context, string, string, string, []byte, any) error {
		return &remenv.TransportError{Err: context.DeadlineExceeded}
	}
	require.NoError(t, f.service.db.Exec("DROP TABLE activities").Error)
	rec := f.request(t, activityListHost, "")
	require.Equal(t, 500, rec.Code, rec.Body.String())
	require.Empty(t, rec.Header().Get(activityListReason))
}

func TestActivityListHeaderContractIsEndpointSpecific(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	paths := f.api.OpenAPI().Paths
	list := paths["/environments/{id}/activities"].Get.Responses["200"]
	header := list.Headers[activityListReason]
	require.NotNil(t, header)
	require.False(t, header.Required)
	require.Equal(t, []any{"agent.unavailable"}, header.Schema.Enum)
	require.Contains(t, header.Schema.Description, "Any marked page makes the whole host read incomplete")
	for path, item := range paths {
		if path == "/environments/{id}/activities" {
			continue
		}
		if item.Get != nil {
			for _, response := range item.Get.Responses {
				require.NotContains(t, response.Headers, activityListReason, path)
			}
		}
	}
}

func TestActivityListDeniedCallerDoesNotReadSource(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	calls := 0
	f.handler.environment.ProxyJSONRequest = func(context.Context, string, string, string, []byte, any) error { calls++; return nil }
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Denied activity read", "0.0.0"))
	RegisterActivities(api, f.handler)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/environments/"+activityListHost+"/activities", nil))
	require.Equal(t, 403, rec.Code)
	require.Empty(t, rec.Header().Get(activityListReason))
	require.Zero(t, calls)
}

func TestActivityListLaterPageFailures(t *testing.T) {
	for _, mode := range []string{"timeout", "authorization", "invalid pagination"} {
		t.Run(mode, func(t *testing.T) {
			var starts []string
			server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) {
				starts = append(starts, r.URL.Query().Get("start"))
				if len(starts) == 1 {
					rows := make([]activity.Activity, remoteActivityPageCeiling)
					for i := range rows {
						rows[i] = remoteActivityListRow()
						rows[i].ID = fmt.Sprintf("agent-run-%d", i)
					}
					writeActivityAgentPage(t, w, rows, 101)
					return
				}
				switch mode {
				case "timeout":
					<-r.Context().Done()
				case "authorization":
					http.Error(w, "example rejection", 403)
				default:
					_, _ = w.Write([]byte(`{"data":[],"pagination":{"totalItems":"invalid"}}`))
				}
			})
			client := server.Client()
			client.Timeout = time.Second
			f := newActivityListFixture(t, false, server.URL, client)
			older := f.seed(t, activityListHost)
			f.seed(t, activityListHost)
			rec := f.request(t, activityListHost, "?start=1&limit=100")
			require.Equal(t, []string{"0", "100"}, starts)
			if mode == "timeout" {
				page := assertActivityListPage(t, rec, "agent.unavailable", []activity.Activity{older})
				require.Equal(t, int64(2), page.Pagination.TotalItems)
			} else {
				require.Equal(t, map[string]int{"authorization": 403, "invalid pagination": 500}[mode], rec.Code, rec.Body.String())
				require.Empty(t, rec.Header().Get(activityListReason))
			}
		})
	}
}

func TestActivityListDirectCallerCancellation(t *testing.T) {
	entered := make(chan struct{})
	server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() })
	f := newActivityListFixture(t, false, server.URL, server.Client())
	f.seed(t, activityListHost)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		f.mux.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/environments/"+activityListHost+"/activities", nil))
		done <- rec
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("activity transport not reached")
	}
	cancel()
	select {
	case rec := <-done:
		require.GreaterOrEqual(t, rec.Code, 400, rec.Body.String())
		require.Empty(t, rec.Header().Get(activityListReason))
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled read did not return")
	}
}
