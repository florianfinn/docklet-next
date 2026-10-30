// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package environment

import "time"

type AgentRestartResponse struct {
	InstanceID string `json:"instanceId"`
}

// AgentRestartRun is bound to a hub registration and credential revision.
type AgentRestartRun struct {
	RunID     string     `json:"runId"`
	State     string     `json:"state" enum:"reserved,sent,awaiting,connected,failed,unknown"`
	CreatedAt time.Time  `json:"createdAt"`
	Deadline  time.Time  `json:"deadline"`
	SettledAt *time.Time `json:"settledAt,omitempty"`
	Code      string     `json:"code,omitempty"`
}
type HostProblem struct {
	Code   string `json:"code" enum:"agent.restart-unsupported,agent.restart-superseded,agent.restart-timeout,agent.restart-unconfirmed,host.busy,host.restarting,host.name-invalid,host.name-taken,host.unreachable,edge.heartbeat-stale,agent.ui-configuration-disabled,settings.ui-configuration-disabled"`
	Status int    `json:"status"`
}

func (p *HostProblem) Error() string  { return p.Code }
func (p *HostProblem) GetStatus() int { return p.Status }
