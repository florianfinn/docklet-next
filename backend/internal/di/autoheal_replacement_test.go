// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original real Auto Heal provider and Docker inspect label propagation proof.

package di

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestAutoHealProviderCarriesInspectedReplacementLabel(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "autoheal.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	localSettings, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, localSettings.Stop(context.WithoutCancel(t.Context()))) })
	require.NoError(t, localSettings.SetBoolSetting(t.Context(), "autoHealEnabled", true))
	run, id := uuid.NewString(), strings.Repeat("d", 64)
	var restarts, inspects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": id, "Names": []string{"/app"}, "State": "running"}})
		case strings.HasSuffix(r.URL.Path, "/containers/"+id+"/json"):
			inspects.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/app", "Config": map[string]any{"Labels": map[string]string{"io.docklet.run": run}, "Healthcheck": map[string]any{"Test": []string{"CMD", "true"}}}, "State": map[string]any{"Running": true, "Health": map[string]string{"Status": "unhealthy"}, "StartedAt": "2026-11-13T12:00:00Z"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restart"):
			restarts.Add(1)
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/version"):
			_ = json.NewEncoder(w).Encode(map[string]string{"Version": "28.0.0", "ApiVersion": "1.48", "MinAPIVersion": "1.41"})
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = w.Write([]byte("OK"))
		default:
			t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	apiClient, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer apiClient.Close()
	cfg := &config.Config{Environment: config.AppEnvironmentTest}
	dockerService := docker.NewDockerClientService(t.Context(), db, cfg, localSettings).WithClient(apiClient)
	activities := activity.NewActivityService(db, localSettings)
	runtime := activity.NewRuntimeService(activities, activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return activity.RuntimeResolvedTarget{Target: target}, nil
	}))
	generation, err := runtime.InitializeGeneration(t.Context())
	require.NoError(t, err)
	now := time.Now().UTC()
	row := activity.RuntimeReceipt{RunID: run, Kind: "receipt", EnvironmentID: "0", StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration", RequestDigest: strings.Repeat("a", 64), CallerID: new("example-owner"), CallerName: new("Operator"), OperationID: new("recreate-container"), TargetKind: new("container"), TargetID: new(strings.Repeat("a", 64)), TargetName: new("app"), GateScope: new("container:" + strings.Repeat("a", 64)), GateKey: new("recreate"), AdmittedAt: &now, DispatchState: new("pending"), ExecutorState: new("executing"), ExecutorStartedAt: &now, FinalWriteAt: now, OutputNextSeq: 1}
	require.NoError(t, db.Create(&row).Error)
	framework := francistest.New(t)
	gate := runs.NewAdmission(framework.Service(), t.Name())
	require.NoError(t, gate.Register(framework))
	francistest.Start(t, framework)
	volumes := volume.NewVolumeService(db, dockerService, nil, activities, localSettings, nil, nil, nil, cfg, nil)
	job, err := provideAutoHealJobInternal(dockerService, localSettings, nil, nil, gate, db, cfg, runtime, volumes)
	require.NoError(t, err)
	_, err = job.Run(t.Context())
	require.NoError(t, err)
	require.Positive(t, inspects.Load(), "provider test must reach the real fresh inspect")
	require.Zero(t, restarts.Load(), "real Busy wiring must carry the inspected replacement label")
	require.NoError(t, db.Model(&row).Where("run_id=?", run).UpdateColumns(map[string]any{"executor_state": "failed", "executor_result_code": string(a.CodeRuntimeFailed), "executor_error": "synthetic settled failure", "executor_settled_at": now, "settled_at": now}).Error)
	_, err = job.Run(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, restarts.Load(), "real provider must restart once after settlement")
}
