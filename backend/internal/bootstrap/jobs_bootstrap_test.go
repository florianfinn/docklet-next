// Source: getarcaneapp/arcane/backend/internal/bootstrap/jobs_bootstrap_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package bootstrap

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/features"
	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
)

type settingsSubscriptionStubInternal struct {
	featureCallbacks []func([]libarcane.SettingUpdate)
	pollingCallback  func([]libarcane.SettingUpdate)
	timeoutCallback  func([]libarcane.SettingUpdate)
}

func (s *settingsSubscriptionStubInternal) SubscribeSettingsChanges(keys []string, callback func([]libarcane.SettingUpdate)) func() {
	if slices.Contains(keys, features.VulnerabilityManagementSettingKey) {
		s.featureCallbacks = append(s.featureCallbacks, callback)
	}
	if slices.Contains(keys, "pollingEnabled") {
		s.pollingCallback = callback
	}
	if slices.Contains(keys, "dockerApiTimeout") {
		s.timeoutCallback = callback
	}
	return func() {}
}

type settingsSubscriptionSchedulerStubInternal struct {
	rescheduled chan struct{}
	jobs        []string
}

func (s *settingsSubscriptionSchedulerStubInternal) RescheduleJob(_ context.Context, job schedulertypes.Job) error {
	if job != nil {
		s.jobs = append(s.jobs, job.Name())
	}
	select {
	case <-s.rescheduled:
	default:
		close(s.rescheduled)
	}
	return nil
}

type timeoutSyncEnvironmentStubInternal struct {
	started chan struct{}
}

func (s *timeoutSyncEnvironmentStubInternal) ListRemoteEnvironments(context.Context) ([]environment.Environment, error) {
	return []environment.Environment{{
		ID:   "remote",
		Name: "remote",
	}}, nil
}

func (s *timeoutSyncEnvironmentStubInternal) ProxyRequest(ctx context.Context, _, _, _ string, _ []byte) ([]byte, int, error) {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	<-ctx.Done()
	return nil, 0, ctx.Err()
}

func TestSettingsTimeoutSyncDoesNotBlockOtherEffectsInternal(t *testing.T) {
	lifecycle := fxtest.NewLifecycle(t)

	settings := &settingsSubscriptionStubInternal{}
	localScheduler := &settingsSubscriptionSchedulerStubInternal{rescheduled: make(chan struct{})}
	localEnvironment := &timeoutSyncEnvironmentStubInternal{started: make(chan struct{})}
	require.NoError(t, setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{
		Lifecycle:     lifecycle,
		LifecycleCtx:  t.Context(),
		Config:        &config.Config{},
		Scheduler:     localScheduler,
		Settings:      settings,
		Environment:   localEnvironment,
		PolicyRefresh: func(context.Context) error { close(localScheduler.rescheduled); return nil },
	}))

	timeoutCallbackDone := make(chan struct{})
	go func() {
		settings.timeoutCallback([]libarcane.SettingUpdate{{Key: "dockerApiTimeout", Value: "30"}})
		close(timeoutCallbackDone)
	}()

	select {
	case <-localEnvironment.started:
	case <-time.After(time.Second):
		require.FailNow(t, "timeout sync did not start")
	}
	select {
	case <-timeoutCallbackDone:
	case <-time.After(time.Second):
		require.FailNow(t, "timeout settings callback remained blocked by remote sync")
	}

	settings.pollingCallback([]libarcane.SettingUpdate{{Key: "pollingEnabled", Value: "true"}})
	select {
	case <-localScheduler.rescheduled:
	case <-time.After(time.Second):
		require.FailNow(t, "local settings effect was blocked by remote timeout sync")
	}

	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, lifecycle.Stop(stopCtx))
}

func TestFeatureChangeReschedulesScanAndPatchJobsInternal(t *testing.T) {
	lifecycle := fxtest.NewLifecycle(t)
	settings := &settingsSubscriptionStubInternal{}
	schedulerStub := &settingsSubscriptionSchedulerStubInternal{rescheduled: make(chan struct{})}
	require.NoError(t, setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{
		Lifecycle:         lifecycle,
		LifecycleCtx:      t.Context(),
		Config:            &config.Config{},
		Scheduler:         schedulerStub,
		Settings:          settings,
		VulnerabilityScan: scheduler.NewVulnerabilityScanJob(nil, nil),
		VulnerabilityRisk: scheduler.NewVulnerabilityRiskJob(nil, nil),
		AutoPatch:         scheduler.NewAutoPatchJob(nil, nil),
	}))
	require.Len(t, settings.featureCallbacks, 3)
	for _, callback := range settings.featureCallbacks {
		callback([]libarcane.SettingUpdate{{Key: features.VulnerabilityManagementSettingKey, Value: "false"}})
	}
	require.ElementsMatch(t, []string{scheduler.VulnerabilityScanJobName, scheduler.VulnerabilityRiskJobName, scheduler.AutoPatchJobName}, schedulerStub.jobs)
	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, lifecycle.Stop(stopCtx))
}

func TestAgentsNeverRescheduleLocalVulnerabilityScan(t *testing.T) {
	for _, edge := range []bool{false, true} {
		lifecycle := fxtest.NewLifecycle(t)
		settings := &settingsSubscriptionStubInternal{}
		jobs := &settingsSubscriptionSchedulerStubInternal{rescheduled: make(chan struct{})}
		require.NoError(t, setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{Lifecycle: lifecycle, LifecycleCtx: t.Context(), Config: &config.Config{AgentMode: !edge, EdgeAgent: edge}, Scheduler: jobs, Settings: settings, VulnerabilityScan: scheduler.NewVulnerabilityScanJob(nil, nil), VulnerabilityRisk: scheduler.NewVulnerabilityRiskJob(nil, nil), AutoPatch: scheduler.NewAutoPatchJob(nil, nil)}))
		for _, callback := range settings.featureCallbacks {
			callback([]libarcane.SettingUpdate{{Key: features.VulnerabilityManagementSettingKey, Value: "true"}})
		}
		require.NotContains(t, jobs.jobs, scheduler.VulnerabilityScanJobName)
		require.Contains(t, jobs.jobs, scheduler.VulnerabilityRiskJobName)
		require.NoError(t, lifecycle.Stop(t.Context()))
	}
}

func (s *settingsSubscriptionSchedulerStubInternal) RegisterJob(job schedulertypes.Job) error {
	s.jobs = append(s.jobs, job.Name())
	return nil
}
func TestVulnerabilityRegistrationIsIntelOnlyOnAgents(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		controller := &settingsSubscriptionSchedulerStubInternal{}
		require.NoError(t, registerVulnerabilityJobs(controller, cfg, scheduler.NewVulnerabilityScanJob(nil, nil), scheduler.NewVulnerabilityRiskJob(nil, nil), &scheduler.VulnerabilityNotifyJob{}))
		require.Contains(t, controller.jobs, scheduler.VulnerabilityRiskJobName)
		if cfg.AgentMode || cfg.EdgeAgent {
			require.NotContains(t, controller.jobs, scheduler.VulnerabilityScanJobName)
			require.NotContains(t, controller.jobs, "vulnerability-notify")
		} else {
			require.Contains(t, controller.jobs, scheduler.VulnerabilityScanJobName)
			require.Contains(t, controller.jobs, "vulnerability-notify")
		}
	}
}
