// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

package activity

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
)

func TestActivitySummaryOptInAndTraceParity(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	item := f.seed(t, "0")
	for _, include := range []bool{false, true} {
		page, err := f.handler.ListActivities(t.Context(), &ListActivitiesInput{EnvironmentID: "0", Limit: -1, IncludeSummary: include})
		require.NoError(t, err)
		require.Len(t, page.Body.Data, 1)
		if !include {
			require.Nil(t, page.Body.Data[0].StepTrace)
			continue
		}
		detail, err := f.service.GetActivityDetail(t.Context(), "0", item.ID, 500)
		require.NoError(t, err)
		require.Equal(t, detail.StepTrace, page.Body.Data[0].StepTrace)
		encoded, err := json.Marshal(page.Body.Data[0])
		require.NoError(t, err)
		require.NotContains(t, string(encoded), `"output"`)
		require.NotContains(t, string(encoded), `"messages"`)
	}
}

func TestActivitySummaryRemoteOptionalTraceSurvives(t *testing.T) {
	for _, trace := range [][]activitytypes.StepTraceEntry{nil, {}} {
		f := newActivityListFixture(t, false, "https://agent.example.com", nil)
		f.handler.environment.ProxyJSONRequest = func(_ context.Context, _, _, path string, _ []byte, out any) error {
			query, err := url.Parse(path)
			require.NoError(t, err)
			require.Equal(t, "true", query.Query().Get("includeSummary"))
			page := out.(*base.Paginated[activitytypes.ActivitySummary])
			page.Success = true
			page.Data = []activitytypes.ActivitySummary{{Activity: remoteActivityListRow(), StepTrace: trace}}
			page.Pagination.TotalItems = 1
			return nil
		}
		rec := f.request(t, activityListHost, "?includeSummary=true&limit=-1")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var page base.Paginated[activitytypes.ActivitySummary]
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
		require.Len(t, page.Data, 1)
		require.Equal(t, trace, page.Data[0].StepTrace)
		require.Equal(t, activityListHost, page.Data[0].EnvironmentID)
	}
}

func TestActivitySummaryChunksAndTransitionWindow(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	items := make([]activitytypes.Activity, 1101)
	for i := range items {
		items[i] = activitytypes.Activity{ID: fmt.Sprintf("summary-%d", i), EnvironmentID: "0", Status: activitytypes.StatusRunning}
	}
	model := Activity{EnvironmentID: "0", Type: activitytypes.TypeProjectDeploy, Status: activitytypes.StatusRunning}
	model.ID = items[0].ID
	require.NoError(t, f.service.db.Create(&model).Error)
	var messages []ActivityMessage
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 600; i++ {
		m := ActivityMessage{ActivityID: items[0].ID, Step: fmt.Sprintf("phase-%d", i/2), Code: "activity.message"}
		m.ID = fmt.Sprintf("step-%04d", i)
		m.CreatedAt = at.Add(time.Duration(i) * time.Second)
		messages = append(messages, m)
	}
	require.NoError(t, f.service.db.CreateInBatches(messages, 100).Error)
	queries := 0
	require.NoError(t, f.service.db.Callback().Row().After("gorm:row").Register("summary-chunk-proof", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "LAG(step)") {
			queries++
			require.LessOrEqual(t, len(tx.Statement.Vars), activityProjectionChunk+1)
		}
	}))
	summaries, err := f.service.listSummariesInternal(t.Context(), items, true, nil)
	require.Equal(t, 12, queries)
	require.NoError(t, err)
	require.Len(t, summaries, 1101)
	require.Len(t, summaries[0].StepTrace, 256)
	require.Equal(t, "phase-44", summaries[0].StepTrace[0].Name)
	require.Equal(t, "phase-299", summaries[0].StepTrace[255].Name)
	require.NotNil(t, summaries[1100].StepTrace)
	require.Empty(t, summaries[1100].StepTrace)
}

func TestActivityForwardingBudgetPrecedesBrowserDeadline(t *testing.T) {
	for _, detail := range []bool{false, true} {
		f := newActivityListFixture(t, false, "https://agent.example.com", nil)
		retained := f.seed(t, activityListHost)
		f.handler.environment.ProxyJSONRequest = func(ctx context.Context, _, _, _ string, _ []byte, _ any) error {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "activity forwarding must own a budget independent of the general proxy")
			require.LessOrEqual(t, time.Until(deadline), 30*time.Second)
			require.Less(t, time.Until(deadline)+5*time.Second, 40*time.Second)
			return context.DeadlineExceeded
		}
		if detail {
			_, err := f.handler.proxyGetActivityInternal(t.Context(), &GetActivityInput{EnvironmentID: activityListHost, ActivityID: "agent-run"})
			require.Error(t, err)
		} else {
			rec := f.request(t, activityListHost, "?includeSummary=true&limit=-1")
			page := assertActivityListPage(t, rec, "agent.unavailable", []activitytypes.Activity{retained})
			require.Len(t, page.Data, 1)
		}
	}
}

func TestActivitySummaryRuntimeTraceAndRecoveryParity(t *testing.T) {
	for _, remote := range []bool{false, true} {
		f := b1Fixture(t)
		require.NoError(t, f.service.BindActivityOwner())
		c := f.command()
		if remote {
			c.Store.EnvironmentID = "example-remote"
			c.Store.ExecutorGeneration = "example-executor"
		}
		id := b3Admit(t, f, c)
		at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		steps := []activitytypes.StepTraceEntry{{ID: "retained-step", Name: "retained executor phase", Status: activitytypes.StatusRunning, StartedAt: at}}
		require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).Update("runtime_step_trace", RuntimeStepTrace(steps)).Error)
		var messages []ActivityMessage
		for i, name := range []string{"pull", strings.Repeat("x", 4097), "start"} {
			m := ActivityMessage{ActivityID: id, Step: name, Level: activitytypes.MessageLevelInfo}
			m.ID = fmt.Sprintf("summary-runtime-step-%d", i)
			m.CreatedAt = at.Add(time.Duration(i) * time.Second)
			messages = append(messages, m)
		}
		require.NoError(t, f.store.svc.db.Create(&messages).Error)
		if remote {
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, c, id), runtimeEvidenceInternal{State: "unknown", UnknownReason: new("transport")})
				return err
			})
		}
		items, _, err := f.store.svc.ListActivitiesPaginated(t.Context(), c.Store.EnvironmentID, pagination.QueryParams{Limit: -1})
		require.NoError(t, err)
		summaries, err := f.store.svc.listSummariesInternal(t.Context(), items, true, nil)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
		require.NotNil(t, summaries[0].Recovery)
		if remote {
			require.Equal(t, new("transport"), summaries[0].Recovery.UnknownReason)
		} else {
			require.Nil(t, summaries[0].Recovery.UnknownReason)
		}
		var row RuntimeReceipt
		require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", id).Error)
		detail, err := runtimeDetailSnapshotTxInternal(f.store.svc.db.DB, &row, false)
		require.NoError(t, err)
		require.Equal(t, detail.StepTrace, summaries[0].StepTrace)
		if remote {
			require.Equal(t, steps, summaries[0].StepTrace)
		} else {
			require.Len(t, summaries[0].StepTrace, 2)
		}
	}
}

func TestActivitySummarySchemaLeavesOtherCarriersUnchanged(t *testing.T) {
	f := newActivityListFixture(t, false, "https://agent.example.com", nil)
	schemas := f.api.OpenAPI().Components.Schemas.Map()
	var summary *huma.Schema
	for name, schema := range schemas {
		if f.api.OpenAPI().Components.Schemas.TypeFromRef("#/components/schemas/"+name) == reflect.TypeFor[activitytypes.ActivitySummary]() {
			summary = schema
		}
	}
	require.NotNil(t, summary)
	require.NotContains(t, summary.Required, "outcomeState")
	require.NotContains(t, summary.Required, "cancelRequested")
	require.NotContains(t, summary.Required, "stepTrace")
	require.NotContains(t, summary.Required, "recovery")
	for _, carrier := range []reflect.Type{reflect.TypeFor[activitytypes.Activity](), reflect.TypeFor[activitytypes.Detail](), reflect.TypeFor[activitytypes.StreamEvent](), reflect.TypeFor[activitytypes.RuntimeSnapshot]()} {
		schema := f.api.OpenAPI().Components.Schemas.Schema(carrier, false, "")
		require.NotContains(t, schema.Properties, "recovery")
		if carrier != reflect.TypeFor[activitytypes.Detail]() {
			require.NotContains(t, schema.Properties, "stepTrace")
		}
	}

}

func TestActivitySummaryTraceBounds(t *testing.T) {
	trace := make([]activitytypes.StepTraceEntry, 300)
	for i := range trace {
		trace[i] = activitytypes.StepTraceEntry{ID: fmt.Sprint(i), Name: strings.Repeat("x", 1024), Status: activitytypes.StatusRunning}
	}
	bounded, err := boundedSummaryTraceInternal(trace)
	require.NoError(t, err)
	encoded, err := json.Marshal(bounded)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), 65536)
	require.LessOrEqual(t, len(bounded), 256)
	require.Equal(t, "299", bounded[len(bounded)-1].ID)
	trace = trace[:300]
	for i := range trace {
		trace[i].Name = "phase"
	}
	bounded, err = boundedSummaryTraceInternal(trace)
	require.NoError(t, err)
	require.Len(t, bounded, 256)
	require.Equal(t, "44", bounded[0].ID)
}

func TestActivityAgentBudgetRetainsColdReceipt(t *testing.T) {
	f := b1Fixture(t)
	require.NoError(t, f.service.BindActivityOwner())
	c := f.command()
	c.Store.EnvironmentID = "example-remote"
	c.Store.ExecutorGeneration = "example-executor"
	id := b3Admit(t, f, c)
	b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, c, id), runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
		return err
	})
	browser := newRuntimeDeadlineFixture(t)
	browser.deadline = time.Now().Add(40 * time.Second)
	h := NewHandler(f.store.svc, EnvironmentDependencies{ProxyJSONRequest: func(agent context.Context, _, _, _ string, _ []byte, _ any) error {
		// Advance the application clock to whichever boundary the actual forwarding
		// context exposes. A missing activity budget lets the browser expire first.
		deadline, ok := agent.Deadline()
		if !ok || deadline.After(time.Now().Add(35*time.Second)) {
			browser.finish(context.DeadlineExceeded)
		}
		return context.DeadlineExceeded
	}})
	page, err := h.ListActivities(browser, &ListActivitiesInput{EnvironmentID: c.Store.EnvironmentID, Limit: -1, IncludeSummary: true})
	require.NoError(t, err, "cold receipt rows must arrive before the browser expires")
	require.NoError(t, browser.Err())
	require.Equal(t, "agent.unavailable", page.HostReason)
	require.Len(t, page.Body.Data, 1)
	require.Equal(t, id, page.Body.Data[0].ID)
	require.Equal(t, new("executor-unavailable"), page.Body.Data[0].Recovery.UnknownReason)
}

func TestActivitySummaryFullWindowLocalDirectEdge(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			rows := make([]activitytypes.Activity, 460)
			for i := range rows {
				rows[i] = remoteActivityListRow()
				rows[i].ID = fmt.Sprintf("window-%d", i)
			}
			calls := 0
			agent := func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "-1", r.URL.Query().Get("limit"))
				require.Equal(t, "true", r.URL.Query().Get("includeSummary"))
				writeActivityAgentPage(t, w, rows, 460)
			}
			endpoint := "https://agent.example.com"
			var client *http.Client
			if mode == "direct" {
				server := serveActivityAgent(t, agent)
				endpoint = server.URL
				client = server.Client()
			}
			f := newActivityListFixture(t, mode == "edge", endpoint, client)
			host := activityListHost
			if mode == "edge" {
				_, _ = connectActivityListEdge(t, agent)
			}
			if mode == "local" {
				host = "0"
				models := make([]Activity, 460)
				for i, row := range rows {
					models[i] = Activity{EnvironmentID: "0", Type: row.Type, Status: row.Status, StartedAt: row.StartedAt}
					models[i].ID = row.ID
				}
				require.NoError(t, f.service.db.CreateInBatches(models, 100).Error)
			}
			rec := f.request(t, host, "?limit=-1&includeSummary=true")
			require.Equal(t, 200, rec.Code, rec.Body.String())
			var page base.Paginated[activitytypes.ActivitySummary]
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
			require.Len(t, page.Data, 460)
			require.EqualValues(t, 460, page.Pagination.TotalItems)
			if mode == "local" {
				require.Zero(t, calls)
				require.NotNil(t, page.Data[0].StepTrace)
			} else {
				require.Equal(t, 1, calls)
				require.Nil(t, page.Data[0].StepTrace)
			}
		})
	}
}

func TestActivitySummaryReceiptProjectionChunks(t *testing.T) {
	f := b1Fixture(t)
	require.NoError(t, f.service.BindActivityOwner())
	items := make([]activitytypes.Activity, 1101)
	for i := range items {
		items[i] = activitytypes.Activity{ID: fmt.Sprintf("receipt-query-%d", i), EnvironmentID: "0"}
	}
	queries := 0
	require.NoError(t, f.store.svc.db.Callback().Query().After("gorm:query").Register("receipt-chunk-proof", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "runtime_receipts") {
			queries++
			require.LessOrEqual(t, len(tx.Statement.Vars), 100)
		}
	}))
	require.NoError(t, f.store.svc.projectRuntimeListInternal(t.Context(), items))
	require.Equal(t, 12, queries)
}

func TestActivitySummaryRetainsCommonTraceBeforeDetailBodyPressureTrim(t *testing.T) {
	f := b1Fixture(t)
	require.NoError(t, f.service.BindActivityOwner())
	c := f.command()
	id := b3Admit(t, f, c)
	b3Phase(t, f, c, id, "executing")
	services := make([]string, 14000)
	for i := range services {
		services[i] = fmt.Sprintf("service-%05d-%040d", i, i)
	}
	output := RuntimeOutputLines{{OutputLine: activitytypes.OutputLine{Timestamp: f.store.now, Stream: "stdout", Text: strings.Repeat("x", 235000)}, OutputSeq: 1}}
	require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).Updates(map[string]any{"affected_services": servicesJSONInternal(services), "runtime_output": output}).Error)
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", id).Update("output_next_seq", 2).Error)
	messages := make([]ActivityMessage, 256)
	for i := range messages {
		messages[i] = ActivityMessage{ActivityID: id, Step: fmt.Sprintf("phase-%03d-%s", i, strings.Repeat("x", 512)), Code: "activity.message", Level: activitytypes.MessageLevelInfo, Message: "phase"}
		messages[i].ID = fmt.Sprintf("body-step-%03d", i)
		messages[i].CreatedAt = f.store.now.Add(time.Duration(i) * time.Second)
	}
	require.NoError(t, f.store.svc.db.CreateInBatches(messages, 100).Error)
	items, _, err := f.store.svc.ListActivitiesPaginated(t.Context(), "0", pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	summaries, err := f.store.svc.listSummariesInternal(t.Context(), items, true, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	var row RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", id).Error)
	detail, err := runtimeDetailSnapshotTxInternal(f.store.svc.db.DB, &row, false)
	require.NoError(t, err)
	require.NotEmpty(t, detail.StepTrace)
	require.Less(t, len(detail.StepTrace), len(summaries[0].StepTrace))
	require.Empty(t, detail.Messages)
	require.Equal(t, summaries[0].StepTrace[len(summaries[0].StepTrace)-1], detail.StepTrace[len(detail.StepTrace)-1])
	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), activitytypes.RuntimeMaxBodyBytes-1024)
	require.Len(t, detail.Output, 1)
	require.Equal(t, output[0].Text, detail.Output[0].Text)
}
