// Source: docklet-next/issues/571
// Revision: 723ece0231761c87611eef58d8f7e0a44c759ea1
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original repository check worker failure trace and producer-shaped web fixture.

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// runRepositoryCheckWorker starts the real check worker for the host-local target and returns its finished detail.
func runRepositoryCheckWorker(t *testing.T, f *runtimePlan, settings map[string]string) *activitytypes.Detail {
	t.Helper()
	require.NoError(t, f.service.deps.DB.AutoMigrate(activity.Models()...))
	f.service.deps.Activity = activity.NewActivityService(f.service.deps.DB, nil)
	for key, value := range settings {
		require.NoError(t, f.service.deps.DB.Exec("INSERT INTO settings (key,value) VALUES (?,?)", key, value).Error)
	}
	ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
	operation, err := f.service.startRepositoryCheck(ctx, "local")
	require.NoError(t, err)
	var detail *activitytypes.Detail
	require.Eventually(t, func() bool {
		detail, err = f.service.deps.Activity.GetActivityDetail(ctx, "0", operation.ActivityID, 50)
		return err == nil && detail.Activity.Status != activitytypes.StatusRunning && detail.Activity.Status != activitytypes.StatusQueued
	}, 20*time.Second, 20*time.Millisecond)
	return detail
}

func traceAndOutput(detail *activitytypes.Detail) (steps, texts []string) {
	for _, entry := range detail.StepTrace {
		steps = append(steps, entry.Name)
	}
	for _, line := range detail.Output {
		texts = append(texts, line.Text)
	}
	return steps, texts
}

func TestRepositoryCheckWorkerPersistsTheFailingStageLast(t *testing.T) {
	present := func(context.Context, string, string, []string) (string, string, error) { return "present", "", nil }
	for _, tc := range []struct {
		name    string
		arrange func(*runtimePlan)
		want    []string
	}{
		{"helper image unavailable", func(f *runtimePlan) {
			f.service.deps.HelperImage = func(context.Context, *client.Client) (string, error) {
				return "", errors.New("synthetic helper unavailable")
			}
		}, []string{"connect"}},
		{"write probe refuses", func(f *runtimePlan) {
			f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) {
				return "", "", errors.New("synthetic denied")
			}
		}, []string{"connect", "save"}},
		{"stored key does not open", func(f *runtimePlan) { f.service.deps.Exec = present; f.exitFor = map[string]int64{"snapshots": 1} }, []string{"connect", "save", "open"}},
		{"integrity check fails after open", func(f *runtimePlan) { f.service.deps.Exec = present; f.exitFor = map[string]int64{"check": 1} }, []string{"connect", "save", "open", "check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimePlan(t)
			tc.arrange(f)
			detail := runRepositoryCheckWorker(t, f, nil)
			require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
			steps, texts := traceAndOutput(detail)
			require.Equal(t, tc.want, steps)
			want := []string{}
			for _, name := range tc.want {
				want = append(want, repositoryCheckStages[name])
			}
			require.Equal(t, want, texts, "raw output ends at the failing stage and claims no count")
			check, _ := detail.Activity.Metadata["backupTargetCheck"].(map[string]any)
			require.Equal(t, false, check["ok"])
			require.Nil(t, check["snapshotCount"])
		})
	}
}

var (
	producerTimestamp = regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T[0-9:.]+(Z|[+-]\d{2}:\d{2})"`)
	producerDuration  = regexp.MustCompile(`"durationMs":\d+`)
	producerID        = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

// The web reader is tested against these producer bytes: a hub-local check and an agent check after the hub
// assigned it an identity (hubHostId); the hub rewrites the agent's environment to that identity on relay.
func TestRepositoryCheckWorkerMetadataWebFixture(t *testing.T) {
	shapes := map[string]any{}
	for name, settings := range map[string]map[string]string{
		"hub":   nil,
		"agent": {"hubHostId": "env-7", "backupTargetSetVersion": "3"},
	} {
		f := newRuntimePlan(t)
		f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) { return "present", "", nil }
		f.stdout = `[{"id":"` + strings.Repeat("a", 64) + `"},{"id":"` + strings.Repeat("b", 64) + `"}]`
		detail := runRepositoryCheckWorker(t, f, settings)
		require.Equal(t, activitytypes.StatusSuccess, detail.Activity.Status, name)
		raw, err := json.Marshal(detail)
		require.NoError(t, err)
		raw = bytes.ReplaceAll(raw, []byte(detail.Activity.ID), []byte("check-run"))
		raw = producerTimestamp.ReplaceAll(raw, []byte(`"2026-11-22T10:00:00Z"`))
		raw = producerDuration.ReplaceAll(raw, []byte(`"durationMs":0`))
		ids := map[string]string{}
		raw = producerID.ReplaceAllFunc(raw, func(id []byte) []byte {
			if _, ok := ids[string(id)]; !ok {
				ids[string(id)] = fmt.Sprintf("%s-m%d", name, len(ids)+1)
			}
			return []byte(ids[string(id)])
		})
		var shape any
		require.NoError(t, json.Unmarshal(raw, &shape))
		shapes[name] = shape
	}
	body, err := json.MarshalIndent(shapes, "", "  ")
	require.NoError(t, err)
	body = append(body, '\n')
	path := "../../../../../web/src/data/http/testdata/repository-check-activity.json"
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(path, body, 0644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, body), "web fixture differs from producer bytes; regenerate with UPDATE_WEB_FIXTURES=1")
}
