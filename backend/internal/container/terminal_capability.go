// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D4 shell availability reader without origin restrictions.

package container

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/container"
)

// ShellCapabilityReader reads current process state after authorization. Standby
// is eligible: the existing proxy opens its on-demand Edge data plane for the read.
func (s *ContainerService) ShellCapabilityReader(hosts *environment.EnvironmentService) containershell.StateReader {
	return func(ctx context.Context, env, containerID string) string {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		running := false
		if env == "0" {
			if s == nil || s.dockerService == nil {
				return "capability.unavailable"
			}
			info, err := s.GetContainerByID(ctx, containerID)
			if err != nil {
				return "capability.unavailable"
			}
			running = info.State != nil && info.State.Running
		} else {
			if hosts == nil {
				return "agent.unavailable"
			}
			host, err := hosts.GetEnvironmentByID(ctx, env)
			if err != nil || !host.Enabled {
				return "agent.unavailable"
			}
			switch host.Status {
			case string(environment.EnvironmentStatusTokenRejected):
				return "agent.invalid-token"
			case "update-required":
				return "agent.update-required"
			case "offline", "error", "pending":
				return "agent.unavailable"
			}
			var response base.ApiResponse[dto.Details]
			if err := hosts.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/containers/"+url.PathEscape(containerID), nil, &response); err != nil {
				var status *remenv.StatusError
				if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
					// A container removed meanwhile is not a host failure; hub-local reads agree.
					return "capability.unavailable"
				}
				_, code := remenv.AgentAccessFailure(err)
				return code
			}
			if !response.Success {
				return "agent.unavailable"
			}
			running = response.Data.State.Running
		}
		if !running {
			return "container.shell-not-running"
		}
		return ""
	}
}
