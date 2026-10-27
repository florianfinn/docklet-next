// Source: getarcaneapp/arcane/backend/internal/job/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package job owns background-job schedule configuration and its HTTP surface.
package job

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	service     *JobService
	environment *environment.EnvironmentService
}

func New(service *JobService, localEnvironment *environment.EnvironmentService) *Module {
	return &Module{service: service, environment: localEnvironment}
}

func (m *Module) Service() *JobService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterJobSchedules(api, nil, nil)
		return
	}
	RegisterJobSchedules(api, m.service, m.environment)
}

func RegisterJobSchedules(api huma.API, jobSvc *JobService, envSvc *environment.EnvironmentService) {
	registerImagePolicyScheduleRoutes(api, jobSvc)
	h := &JobSchedulesHandler{
		jobService:      jobSvc,
		proxyRemoteJSON: envSvc.ProxyJSONRequest,
	}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-job-schedules",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/job-schedules",
		Summary:     "Get job schedules",
		Description: "Get configured cron schedules for background jobs",
		Tags:        []string{"JobSchedules"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermJobsManage, h.Get)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "update-job-schedules",
		Method:      http.MethodPut,
		Path:        "/environments/{id}/job-schedules",
		Summary:     "Update job schedules",
		Description: "Update background job cron schedules and reschedule running jobs",
		Tags:        []string{"JobSchedules"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermJobsManage, h.Update)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-jobs",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/jobs",
		Summary:     "List all background jobs",
		Description: "Get status, schedule, and metadata for all background jobs",
		Tags:        []string{"JobSchedules"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermJobsManage, h.ListJobs)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "run-job",
		DefaultStatus: http.StatusAccepted,
		Method:        http.MethodPost,
		Path:          "/environments/{id}/jobs/{jobId}/run",
		Summary:       "Run a job now",
		Description:   "Manually trigger a background job to run immediately",
		Tags:          []string{"JobSchedules"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermJobsManage, h.RunJob)
	h.registerRunRoutesInternal(api)
}

func (h *JobSchedulesHandler) registerRunRoutesInternal(api huma.API) {
	basePath := "/environments/{id}/jobs/{jobId}"
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-job-runs",
			Method:      http.MethodGet,
			Path:        basePath + "/runs",
			Summary:     "List job runs",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.ListRuns,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-job-run",
			Method:      http.MethodGet,
			Path:        basePath + "/runs/{runId}",
			Summary:     "Get job run",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.GetRun,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "retry-job-run",
			Method:      http.MethodPost,
			Path:        basePath + "/runs/{runId}/retry",
			Summary:     "Retry job run",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.RetryRun,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "cancel-job-run",
			Method:      http.MethodPost,
			Path:        basePath + "/runs/{runId}/cancel",
			Summary:     "Cancel pending job run",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.CancelRun,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "resolve-job-run",
			Method:      http.MethodPost,
			Path:        basePath + "/runs/{runId}/resolve",
			Summary:     "Resolve legacy job run",
			Description: "Deprecated compatibility endpoint for mixed-version upgrades. Removed in v3.",
			Deprecated:  true,
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.ResolveRun,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "ack-job-run",
			Method:      http.MethodPost,
			Path:        basePath + "/runs/{runId}/ack",
			Summary:     "Acknowledge remote run completion",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.AcknowledgeRun,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "restart-job-worker",
			Method:      http.MethodPost,
			Path:        basePath + "/restart",
			Summary:     "Restart continuous job worker",
			Security:    handlerutil.DefaultOperationSecurity(),
		},
		authz.PermJobsManage,
		h.RestartWorker,
	)
}
