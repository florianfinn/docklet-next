# Agent HTTP surface

The agent and Edge agent register the same reviewed module set. The list in
`backend/api/api.go` is the registration source; agent Huma routes and all Echo
routes are captured by tests. The agent receives privileged host operations
from the hub. The hub validates user permissions before forwarding, and the
agent validates its manager credential. A compromised manager credential could
invoke the host operations below, so additions require an explicit review.

| Module | Host exposure and reason |
| --- | --- |
| health | Readiness endpoint for the hub and local supervisor. |
| version | Public version metadata for protocol compatibility. Only the configured manager credential on an agent adds project-directory ownership, effective write access and socket-group checks; the on-demand write probe is cached for 30 seconds. The hub never includes these checks. |
| project | Reads and changes Compose projects and project files; needed for stack management. |
| container | Reads state, logs and processes and controls lifecycle and configuration. |
| image | Pulls, removes, exports, patches and scans images; needed for image management. |
| image-update | Checks and records available container image updates. |
| volume | Reads, writes, backs up and restores Docker volumes and workspace files. |
| network | Reads topology; validates create, protected deletion and membership changes on the Docker host (agent contract v6). |
| port | Reports host port use for deployment decisions. |
| upload | Accepts bounded uploads used by image and restore operations. |
| settings | Reads and updates environment settings, including host operational configuration. |
| updater | Checks and applies configured host resource updates. |
| system | Reports Docker host state, runs Docker-wide lifecycle/prune operations, and exposes agent self-upgrade checks and trigger. A manager credential can replace the agent binary; hub backup and update-all remain excluded. |
| diagnostics | Exposes host actor diagnostics for support and fault investigation. |
| registry | Accepts registry credential synchronization from the hub for private pulls. Listing, editing and testing credentials are excluded; a manager credential can still replace agent pull credentials. |
| job | Exposes environment-scoped schedules, jobs and run controls for hub orchestration. A manager credential can trigger host jobs, retry runs and alter schedules. |
| activity | Exposes environment-scoped activity history, detail and cancellation. A manager credential can read task output, cancel work and clear history. |
| vulnerability | Scans images and records vulnerability findings and ignore rules. |
| WebSocket routes | Streams container/project logs, stats and shell traffic; the Echo route snapshot protects these non-Huma endpoints. |

Identity, users, roles, API keys, environment enrollment and management,
notifications, webhooks, dashboard, search and hub event ingress
remain hub routes. Agent mode also omits the browser UI, API documentation,
pprof profiling endpoints, test-only Playwright routes and manager Edge tunnel
listener. An Edge agent
connects outward; this allowlist does not open a public Edge listener.
