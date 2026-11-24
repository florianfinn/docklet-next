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
  the maintainer at milestone start; after a foundation follow-up the main thread
  rechecks the plan and reports changes on the milestone closing issue.
- docs/design/ describes decisions and rationale, not task status. Dated measurements
  are evidence. Historical plans belong in the docs/history/ index.

## Agent coordination

- One main thread runs a milestone and is the only writer of its feature branch: it
  plans, writes design notes, implements, fixes and integrates, also for steps a
  tool built. Subagents are tools with one self-contained brief (goal and
  acceptance, full base/head SHA, accepted decisions, owned and read-only paths,
  prior findings and objections) and one structured result. Tools never decide
  product, scope or UX questions, and no subagent changes a running system.
- At milestone start, the plan and the design notes of risky steps (with their
  break maps) get one independent design review, then the maintainer is asked once:
  every open product, scope and interface question with a recommendation, plus the
  acceptance matrix and its exclusion rule. Implementation starts only after the
  maintainer agrees to both. Each exclusion names the affected behaviour and cells, the
  baseline evidence or environment limit, the runner's check and the effect on the
  verdict; code age alone is no exclusion. Later questions are decided by the main
  thread with the recommended, reversible and narrower option and posted with how
  to reverse it on the closing issue; such a decision never widens scope,
  publishes, weakens security, deploys or sets an acceptance verdict. Unresolved
  acceptance-scope or exclusion decisions go to the maintainer; only the affected
  step waits.
- Parallel tools only with independent scope, disjoint ownership and isolated
  worktrees. Group work into coherent deliveries; repeat successful full suites
  only when changes or failures justify it.
- For a correctness fix, map producer, transport/DTO and actual consumer first;
  fixtures follow the real producer. Cover initial failure, partial state, latency,
  identity changes and restart where relevant, and synchronise asynchronous
  assertions with the actual effect. Each break-map row (persisted transitional
  states, external data entry points, identity changes, new codes, Direct/Edge
  parity, migrations) and each blocker fix needs a test with red proof; compiler
  errors, timeouts and equivalent mutations are no red proof.
- Review depth follows risk: foundations, contracts, migrations, auth, restore,
  agent surfaces and the interim-review kinds in docs/design/review-workflow.md get
  an independent review; other steps use self-checks and CI. Implementation and
  review use different providers. After PASS, nonblocking improvements do not
  trigger further review rounds.
- Record flaky failures and reruns honestly and track their repair; never bypass
  required hooks or checks. Worker reports and green local checks are intermediate
  results. Retain real exit codes; after an interruption inspect task, Git and
  remote-ref state before retrying. Before ending a turn with pending work,
  establish an app-owned continuation (a running tool or a PR watch).

## Branches, review and completion

- Read docs/design/review-workflow.md before push, PR creation, review and merge.
- One feature branch per milestone, written only by its main thread; steps arrive
  as pushed commits without step PRs. The draft closing PR opens with the first
  push, so CI runs on every pushed head. The closing
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
  focused tests of the touched modules; the pre-push hook runs the push subset of
  lint (publication, structure, migrations, API contract, Go formatting and web
  typecheck). CI runs the full lint, test and build once per pushed head: on main
  pushes and on PRs, which compare with their base and check the merge result.
  GitHub runs no PR checks while a PR has merge conflicts: merge main into the
  feature branch first and rely only on CI evidence of a mergeable head.
- Text-only changes use scripts/change-scope.mjs: Markdown outside executable
  paths, root LICENSE/NOTICE and LICENSES. A release train that adds only the next
  X.Y.Z version and its provenance hash and note counts as text. Reduced checks still
  cover publication, commit text, structural guards and Node tests, including the
  release tests. Pushes to main run the full CI chain.
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
