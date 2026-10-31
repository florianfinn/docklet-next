// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original agent-only Docker self restart receiver.

package system

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/health"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/moby/moby/client"
	"go.getarcane.app/sys/cgroup"
)

type agentRestartHandler struct {
	client func(context.Context) (*client.Client, error)
	selfID func() (string, error)
	wait   func(context.Context, time.Duration) bool
	appCtx context.Context
}

func registerAgentRestart(api huma.API, dockerService *docker.DockerClientService, appCtx context.Context) {
	h := &agentRestartHandler{selfID: cgroup.CurrentContainerID, wait: waitAgentRestart, appCtx: appCtx}
	if dockerService != nil {
		h.client = dockerService.GetClient
	}
	h.register(api)
}

func (h *agentRestartHandler) register(api huma.API) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "restart-agent", Method: "POST", Path: "/system/agent/restart", DefaultStatus: 202,
		Summary: "Restart the agent container", Tags: []string{"System"},
	}, authz.PermSystemUpgrade, h.restart)
	handlerutil.HostProblemResponses(api, api.OpenAPI().Paths["/system/agent/restart"].Post, "409")
}

func waitAgentRestart(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *agentRestartHandler) restart(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
	unsupported := &dto.HostProblem{Code: "agent.restart-unsupported", Status: 409}
	if h.client == nil || h.selfID == nil {
		return nil, unsupported
	}
	id, err := h.selfID()
	if err != nil || strings.TrimSpace(id) == "" {
		return nil, unsupported
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	dockerClient, err := h.client(probeCtx)
	if err != nil || dockerClient == nil {
		return nil, unsupported
	}
	inspected, err := dockerClient.ContainerInspect(probeCtx, id, client.ContainerInspectOptions{})
	if err != nil || inspected.Container.ID == "" {
		return nil, unsupported
	}
	// Resolve once before acceptance; the delayed effect uses the inspected ID.
	ownID := inspected.Container.ID
	return &huma.StreamResponse{Body: func(response huma.Context) {
		response.SetHeader("Content-Type", "application/json")
		response.SetStatus(202)
		if err := json.NewEncoder(response.BodyWriter()).Encode(dto.AgentRestartResponse{InstanceID: health.InstanceID()}); err != nil {
			return
		}
		if flusher, ok := response.BodyWriter().(interface{ Flush() }); ok {
			flusher.Flush()
		}
		life := h.appCtx
		if life == nil {
			life = context.WithoutCancel(ctx)
		}
		go func() {
			if !h.wait(life, time.Second) {
				return
			}
			effectCtx, cancel := context.WithTimeout(life, 30*time.Second)
			defer cancel()
			if _, err := dockerClient.ContainerRestart(effectCtx, ownID, client.ContainerRestartOptions{}); err != nil {
				slog.WarnContext(effectCtx, "Agent container restart failed", "code", "agent.restart-unconfirmed")
			}
		}()
	}}, nil
}
