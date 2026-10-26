# Docklet Next

Docker and Compose management with a Go hub and agent core and a React interface.

**Status: early development; no product release.** The Go hub and agent core and
React interface build locally. The hub uses SQLite on local persistent storage;
no database service or old-data import is supported. See the
[hub candidate package](deploy/hub/README.md) for storage, restart and recovery.

Based on Arcane. The core is intended for one-time adoption followed by independent
maintenance. Own code is licensed under [Apache-2.0](LICENSE); Arcane-derived parts
retain [BSD-3-Clause](LICENSES/BSD-3-Clause-Arcane.txt). See [NOTICE](NOTICE) and
[licensing](docs/design/licensing.md) for the split and distribution duties.

Start with [CONTRIBUTING.md](CONTRIBUTING.md), the binding [AGENTS.md](AGENTS.md),
and the [design index](docs/design/README.md). Report vulnerabilities privately
as described in [SECURITY.md](SECURITY.md).

The live runtime connects stack start/stop/restart/redeploy and container
start/stop/restart to durable action runs, with caller-scoped retry keys, resource
exclusion and explicit handling of unknown outcomes. Stack and container logs are
live streams with bounded buffering and visible gaps, rather than durable replay.
See the [runtime contract](docs/design/runtime-control.md),
[API mapping](docs/design/api-contract.md) and
[M6 acceptance record](docs/history/m6-runtime-acceptance.md) for boundaries and evidence.

For agent collaboration, follow [Agent coordination](AGENTS.md#agent-coordination)
and [Branches, review and completion](AGENTS.md#branches-review-and-completion).
The separately maintained
[subagent-orchestration skill](https://github.com/florianfinn/claude-skills/tree/master/plugins/subagent-orchestration)
is an optional general reference; project instructions take precedence. Current work belongs in [GitHub issues](https://github.com/florianfinn/docklet-next/issues)
and [milestones](https://github.com/florianfinn/docklet-next/milestones).

The hub uses `TZ` for schedules and date display, defaulting to `UTC`. Configure
an IANA zone in the deployment environment and restart the hub after changes;
General settings show the effective zone read-only.
