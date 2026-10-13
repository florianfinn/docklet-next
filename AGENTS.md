<!--
Source: docklet-hub/AGENTS.md
Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
Retrieved: 2026-10-13
License: Apache-2.0
Deviations: English rewrite; milestone review cut, Go/React boundaries, free components and M0 guards.
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

## Branches, review and completion

- Read docs/design/review-workflow.md before push, PR creation, review and merge.
- One feature branch per milestone; steps arrive by PR. The closing PR integrates
  the complete milestone into main and lists every completed issue with Closes.
- Merge main into feature branches; never rebase or force-push shared history.
  No direct push to main. Push and PR creation require explicit instruction.
- Conventional Commit subjects include (#number) when an issue exists. The
  author and committer use public human noreply identities.
- Squash into main; PR title/body become commit subject/body. Revert titles also
  use Conventional Commits. On base changes, push again to rerun checks.
- One independent review per milestone closing PR, plus risk interim reviews;
  implementation and review use different providers. Step PRs use self-checks and
  CI; standalone PRs and foundation follow-ups receive independent review.
- Completion requires merge to the target branch, green checks and the applicable
  review. Record PR, merge SHA, review and check results in the issue. The milestone
  closing issue stays open until its closing PR merges to main. Practical and release
  acceptance have their own evidence; local tests or a demo do not replace them.

## Checks and boundaries

- Pre-commit checks publication and structure against staged blobs and policies.
- Stage new files before checks. core.hooksPath must be .githooks. Before push run
  pnpm run lint and pnpm run test; CI also builds. The root commands integrate Go
  vet/tests and web lint/typechecks/tests when those modules arrive in M1/M2.
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
- PostgreSQL is the only database. Do not import earlier docklet-hub or Arcane
  databases or registrations. Establish a baseline before the first release;
  shipped migrations are immutable and run forward at startup.
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
