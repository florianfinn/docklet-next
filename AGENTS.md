<!--
Source: docklet-hub/AGENTS.md
Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
Retrieved: 2026-10-13
License: Apache-2.0
Deviations: English rewrite; milestone review cut, Go/React boundaries, free components, M0 guards and SQLite local-storage policy.
-->

# AGENTS.md

Binding rules for people and agents working on Docklet Next. Work status lives
in GitHub issues and milestones; rationale belongs in docs/design/ and its index.

## Language and provenance

- Identifiers, filenames, comments, documentation, issues, PRs, commits and merge
  text are English. Collaboration with the maintainer may be German.
- UI text belongs in feature or platform catalogues. English is the base locale;
  German is complete. Check keys, ICU syntax and matching placeholders; double
  ICU apostrophes. Do not enforce German transliteration or umlaut prose rules.
- Comments describe current contracts, constraints and non-obvious relations,
  usually in one to three lines. History belongs in Git/issues or indexed design
  rationale. Preserve licence notices and necessary tool directives.
- Adopted files record source path, full source SHA or content hash, retrieval
  date, licence and justified deviations. Preserve all existing notices.
- Use free shadcn/ui primitives only. Premium blocks may serve as visual
  references, never as source. No premium registry or credential is required.
- No AI tool is an author, maintainer or co-author. Human authors and committers
  use public noreply addresses; GitHub may be the merge committer. Plain
  Assisted-by text without an email is allowed. Session links are prohibited.

## Public data

- Files, issues, PRs, commits, screenshots and release artefacts contain only
  publishable information. No secrets, private emails, hosts, domains, operational
  IP addresses, network plans, routes or personal workstation paths, even in tests.
- Examples use reserved example domains, RFC 5737 IPv4 or 2001:db8::/32 IPv6.
  Synthetic parser fixtures require exact reviewed exceptions, never directory exemptions.
- Runtime secrets stay in local .env files. Every runtime variable is documented
  in .env.example. The optional operator publication denylist stays local and untracked.
- Check the index before commit and new commit content and identities before push.
  Findings print only path and category. Automation does not replace human review
  of prose and images; every independent review includes publication suitability.
- Only hash-allowlisted assets may be committed. Video, music, render caches and
  screenshots of real environments are excluded. Keep private evidence private.

## Planning and scope

- Establish milestones before feature/release work. Each states goal, scope,
  dependencies, owned module paths, ready gate and acceptance. A closing issue
  carries evidence. No catch-all tracking issues; bounded review follow-up packages
  are allowed under docs/design/review-workflow.md.
- Issues cover features, architecture/product decisions and findings not fixed in
  the session. Small fixes, maintenance, dependencies and rule updates go directly
  through a PR without a new issue or milestone. Feature/release issues belong to
  one milestone and owning module. Maintainer working rules need no decision issue.
- The ready gate names accepted and merged prerequisites with contract versions,
  write paths and read-only neighbours, resolved or explicitly excluded evaluations,
  acceptance tests and practical acceptance. Freeze scope when the milestone starts.
- Record a missing prerequisite as dependency-gap with owner and evidence. Pause
  only the affected issue. Plan it in its owner's milestone or, if already accepted,
  as a foundation follow-up with its own issue, PR and review. Merge that prerequisite
  before resuming; never silently expand the current issue into another module.
- Shared foundations (OpenAPI, run schema, migrations, adapter port, preferences)
  have exactly one owning step, merged before dependent work. Recheck planning with
  the maintainer at milestone start and after foundation follow-ups.
- docs/design/ describes decisions and rationale, not task status. Dated measurements
  are evidence. Historical plans belong in the docs/history/ index.

## Agent coordination

- One main thread runs a milestone: it plans, writes design notes, implements,
  fixes and integrates, and is the only writer of the feature branch. Fix rounds
  stay in the main thread, also for steps a tool built. Subagents are
  tools with one brief and one structured result: independent review, sandbox and
  acceptance runs, wide searches, and large bounded steps in their own worktree,
  which the main thread checks and integrates. Tools never decide product, scope or
  UX questions; they return them with options and a recommendation.
- At milestone start, write the plan and the design notes of risky steps (with
  their break map) and run one bundled design review over them, then ask the
  maintainer every open product, scope and interface question at once, each with a
  recommendation, together with the acceptance matrix and its exclusion rule, and
  obtain agreement to both before implementation. Each exclusion names the affected
  behaviour and matrix cells, the baseline evidence or named environment limit, the
  evidence the runner checks and its effect on the verdict. Code predating the
  milestone is not by itself an exclusion; regressions and required milestone
  behaviour remain subject to acceptance. Read accepted decisions first; handle
  routine implementation choices autonomously. Except for unresolved
  acceptance-scope or exclusion decisions, a question that appears later is decided by the main thread with the recommended, reversible
  and narrower option, recorded with how to reverse it and reported to the
  maintainer without pausing, as a comment on the milestone closing issue; such a
  decision never widens scope, publishes, weakens security, deploys or sets an
  acceptance verdict.
- Give each agent a self-contained brief: goal and acceptance, full base/head SHA,
  target branch, accepted decisions, prerequisites and contract owner, owned paths
  and read-only boundaries, known traps, checks and accessible evidence/report paths.
  Never rely on access to the parent conversation. Review rounds receive prior
  findings, responses and unresolved objections alongside the current exact SHA.
  Put the current context first and link complete relevant reports once through
  verified accessible paths; avoid recursively copying earlier briefs. Preserve
  critical decisions and objections when working within tool input limits.
- Use parallel tools only with independent scope, disjoint ownership and isolated
  worktrees. Group work into coherent,
  usable deliveries; local checkpoints and file-count heuristics do not require
  additional PRs, scouts or review rounds. Retain the required foundation, risk
  and closing reviews. Use focused construction checks and required integrated
  checks; repeat successful full suites only when changes or failures justify it.
- For a correctness fix, map the affected producer, transport/DTO and actual
  consumer before implementation. Cover initial failure, partial state, latency,
  identity changes and restart where relevant. Synchronise asynchronous assertions
  with the actual effect. Each break-map row (persisted transitional states,
  external data entry points, identity changes, new codes, Direct/Edge parity,
  migrations) and each blocker fix needs a test with red proof; other code gets
  ordinary tests. Compiler errors, timeouts and equivalent mutations do not
  demonstrate that a behavioural guard works.
- Check dependency graphs, fixtures, metadata and policy guards early. Record
  flaky failures and reruns honestly and track their repair. Evidence reuse never
  permits bypassing required hooks or checks without an explicit policy change.
- After each review, disposition findings once: blockers, selected related fixes
  grouped into one correction, and tracked deferrals with rationale. After PASS,
  accepted nonblocking improvements do not trigger additional review loops.
  Signal new product questions promptly through the active coordination channel.
- Implementation tools default to GPT-6.1 Sol. Start Astra agents only when
  strictly necessary and state the concrete reason. Review always comes from a
  different provider than the implementation, including main-thread work.
- Continue through the authorised completion boundary. Worker reports and green
  local checks are intermediate results. After interruptions, inspect actual task,
  Git and command results before retrying; preserve the next allowed step in
  handovers. Keep required review, checks, acceptance and external-action approvals.
  Verify the actual mounted worktree, toolchain and environment before execution.
  Retain real exit codes. Before ending a turn with pending work, establish an
  app-owned continuation or PR watch and record its ID and next action. After an
  interrupted push, inspect the remote ref before retrying; do not duplicate work.

## Branches, review and completion

- Read docs/design/review-workflow.md before push, PR creation, review and merge.
- One feature branch per milestone, written only by its main thread; steps arrive
  as pushed commits without step PRs, and CI runs on every pushed head. The closing
  PR integrates the complete milestone into main and lists every completed issue
  with Closes.
- Merge main into feature branches; never rebase or force-push shared history.
  No direct push to main. Push and PR creation require explicit instruction.
- Conventional Commit subjects include (#number) when an issue exists. The
  author and committer use public human noreply identities.
- Squash into main; PR title/body become commit subject/body. Revert titles also
  use Conventional Commits. On base changes, push again to rerun checks.
  Before merge, check the actual proposed squash subject/body, required issue
  reference in the subject, public human identities and publication suitability.
- One independent review per milestone closing PR, plus risk interim reviews of
  the step's commit range; implementation and review use different providers.
  Other steps use self-checks and CI. Standalone PRs and foundation follow-ups
  receive independent review.
- Before the closing review, run the candidate package once in the approved
  sandbox against real hub and agent paths (Direct and Edge where relevant).
  Check fixtures for hub or agent responses against the real producer.
- Completion requires merge to the target branch, green checks and the applicable
  review. Record PR, merge SHA, review and check results in the issue. The milestone
  closing issue stays open until its closing PR merges to main. Practical and release
  acceptance have their own evidence; local tests or a demo do not replace them.
  The acceptance runner applies the agreed matrix and exclusion rule from its brief,
  and its verdict stands. Fix ordinary acceptance defects within the agreed scope
  and rerun. If an unexpected failure needs a scope or exclusion decision the rule
  does not determine, keep the failing verdict and ask the maintainer; only the
  affected acceptance or completion step waits. A main-thread decision or issue
  comment cannot waive a failure or replace the runner's verdict.

## Checks and boundaries

- Pre-commit checks publication and structure against staged blobs and policies.
- Stage new files before checks. core.hooksPath must be .githooks. Before push run
  focused tests of the touched modules; the pre-push hook runs pnpm run lint
  (publication, structure, migrations, Go vet, web lint and typechecks). CI runs the
  full lint, test and build on every pushed head of main and feature/** (feature
  pushes compare with their main merge base) and on PRs; while a closing PR is
  open, a feature push also runs the PR merge-result check.
- Text-only changes use scripts/change-scope.mjs: Markdown outside executable
  paths, root LICENSE/NOTICE and LICENSES. Reduced checks still cover publication,
  commit text, structural guards and Node tests. Pushes to main run the full CI chain.
- CI has minimal permissions and no production secrets. Tests require no real
  services, Docker socket or running agent. Guards fail instead of warning.
  Test real gestures or pure logic; do not change behaviour to appease a test bench.
- Own files stay at or below 1,000 lines. Adopted oversized files have reviewed
  ceilings in scripts/size-baseline.json that may only shrink; split them when
  making substantive changes. First adoption requires a new path under configured
  Arcane adoption roots in provenance.json adoptedPrefixes, a valid header and an
  exact line ceiling. Git-detected renames and copies retain their source ceiling
  and base line count. Provenance coverage alone grants no size exemption. Generated files
  need exact entries matching reviewed lockfile names or generated directories
  with a generated marker; arbitrary entries and stale exemptions fail.
- DOM assertions compare booleans, not cyclic nodes. Open Radix Select with the
  keyboard in happy-dom; do not replace accessible components for test convenience.
- Demo adapters and the design lab exist only in development/tests. A production
  bundle guard must reject them in M2. Production routes, query parameters and
  settings cannot enable simulation. Personal colours, views and design-code
  import/export remain product functions outside the lab.

## Dependencies, images and data

- Justify dependencies and record compatible licences. Prefer small understandable
  helpers. Own code is Apache-2.0; Arcane-derived parts retain BSD-3-Clause notices.
- The Go core runs as hub or agent; agent and edge modes register allowlisted
  modules only. Record each new Go dependency and its licence. Wire vocabulary
  belongs in types/ and checked-in OpenAPI; generate TypeScript DTOs from it.
- Hub, agent, API and UI share a version. Minimum agent and Edge protocol versions
  are explicit release boundaries. Announce protocol breaks and new required values.
- SQLite is the only hub database. Keep database files on local persistent
  storage; network shares are unsupported for them. Do not import earlier
  docklet-hub or Arcane databases or registrations, or development PostgreSQL
  data. Establish a fresh baseline before the first release; shipped migrations
  are immutable and run forward at startup.
- Run only at the root of a dedicated domain or subdomain; sub-path hosting is
  unsupported. This applies to assets, routing, API, WebSockets, auth and links.
- Pin images to SemVer and digest where available; never latest in install templates.
  References are overridable. Local image builds remain a registry-free fallback.

## Deploy and change text

- No subagent changes a running system. Every deploy needs explicit operator
  approval for that deployment and uses origin/main. Back up the database before
  schema changes; verify migrations, /health and the served web bundle afterwards.
- Put deploy risks under Before deploy in the PR. No product release without
  practical acceptance of its exact package in an approved sandbox.
- Lead change text with the concrete change. Give measured counts and methods,
  honest limits and connected explanations; use lists for parallel items.
