<!--
Source: docklet-hub/docs/design/review-workflow.md
Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
Retrieved: 2026-10-13
License: Apache-2.0
Deviations: English rewrite; milestone closing review, risk gates and different-provider requirement.
-->

# Push, review and completion

Read this procedure before push, PR creation, review or merge. One independent
review covers each milestone closing PR. Milestone steps are commits pushed to the
feature branch by its main thread, without step PRs; they use self-checks and the CI
run on each pushed head. Independent interim reviews of the step's commit range
are required for high risk steps: licensing before the first Arcane-code commit; agent surface/allowlist;
database baseline; identity/authentication; restore; simple-user authorisation;
and storage of widget credentials. Standalone maintenance PRs and foundation
follow-ups also receive independent review. A root commit is reviewed as a complete
tree before first publication, with acceptance owned separately from implementation.

## Preparation and review scope

Self-check before the first push. Search for the meaning of removed/changed
behaviour in code, catalogues, docs and comments, not only old identifiers. Verify
new factual claims. Run focused tests; the pre-push hook runs publication,
provenance and lint, and CI runs the full suite. Push and PR
creation still require explicit instruction. Remove generated session-link
footers immediately. PR reviews start after green pr-text; commit-range reviews of
feature-branch steps start after the step head's CI run is green and
`node scripts/check-publication.mjs --history <base>..<head>` passes for the range.

Implementation and independent review use different providers. The reviewer
receives a fresh context: AGENTS.md, this procedure, task, issues, acceptance
criteria, actual target branch, full head SHA and full diff. Review behaviour,
tests, semantic searches, new assertions and publication suitability. Report all
findings together even after finding a blocker; run tests where useful. Mechanical
imports additionally require hash/guard evidence. M0 uses a medium-tier review;
licence and other risk gates use a high-tier review. Choose current equivalent
models for each provider; model choice does not substitute for independence.

## Findings and follow-up rounds

Each finding has a stable ID, location, evidence and justified severity. Blockers
violate behaviour, security or binding acceptance, or materially mislead users.
Minor wording alone is not a blocker. Record each as fixed, rejected with reasons,
or accepted follow-up. Fixed entries name commits; follow-ups name an issue and
stable ID. An unresolved blocker keeps Result: fail; filing an issue cannot waive it.

Fix a blocking error class across comparable paths in one batch, with a sweep
showing no other instance remains and red proof that the regression tests fail
without the correction. Include
related optional fixes, self-check and push together. After pass, defer further
optional improvements instead of rebuilding a reviewed result unnecessarily.

Every changed reviewed head needs fresh independent evidence. Follow-up review
receives the prior full report, decisions, previous SHA, new diff and access to the
whole target diff. It checks fixes, consequences and regressions; unchanged areas
need not be re-reviewed without cause. Broaden scope for large rewrites, missing
reports or new evidence and explain why. Do not reopen accepted optional findings
as blockers without new evidence.

After at most two unsuccessful targeted rounds per finding class, document causes
in a table over every producer, transport and consumer of the affected data,
including code that predates the change, and decide the fix boundary: the whole
class in this change, or the change's own surfaces with the pre-existing remainder
as a tracked follow-up. Later verification judges only that boundary. A third
failing round in the class rebuilds the fix from the table instead of patching
the last finding. Ask the maintainer
only for unresolved product decisions. This limit is not merge permission.
A target-branch change requires a full review and another push for checks. An
unrelated conflict-free update of the same base does not invalidate review evidence.

## Bounded follow-up packages

Search open issues for duplicates before recording optional follow-ups. Group only
related findings with a common correction goal and check path. Each checkbox has a
stable ID, location, evidence, desired outcome and originating PR/review reference.
State goal, scope and acceptance so it can be worked without reconstructing chat.
Sanitise private evidence. Use the owning feature/release milestone where applicable.

When taking work, record selected IDs and owner; the implementing PR names them.
After merge record PR, SHA and check evidence. Partial PRs do not close the whole
package. Close it only when all findings are fixed or rejected with reasons; report
remaining packages explicitly at handoff.

## Evidence and required review status

The maintainer posts a new PR comment with this exact uninterrupted header:

```text
Independent review
Reviewer: reviewer name; provider different from implementer
Head: <full 40-character commit SHA>
Result: pass

Scope, prior review/head if applicable, finding decisions, fixes/issues,
checks with counts, publication review and remaining limits.
```

Use fail when blockers remain. Name a real independent reviewer; never attest to
self-review. The script validates the header and current head, not the truth of
provider independence; the maintainer verifies that claim against the report.
Only newly created OWNER comments beginning with Independent review count.
Edits do not trigger acceptance; post a new comment. Malformed, stale and failing
reviews set failure on the current PR head, replacing any earlier passing status.
A new head starts pending and requires fresh evidence. Closed PRs receive no status.

review-status.yml uses trusted default-branch code only and the automatic GitHub
token with contents/pull-requests read and statuses write. Never execute PR code
in that workflow. Regular checks/pr-text run without write permission or production
secrets. External contributor workflows require maintainer approval.

## Merge and limits

The main ruleset must require PRs and checks, pr-text and review, without strict
base freshness. Installing that ruleset and enabling private vulnerability reports
are maintainer acceptance tasks; local files do not configure GitHub. Feature-branch
steps rely on self-checks/CI; closing and standalone PRs also require review evidence.
Interim risk reviews record their result, commit range and finding decisions in the
closing PR body.

Squash the closing PR only when the current head is green, its independent result
is pass and no blockers remain. The title/body become the commit subject/body;
include every completed issue with Closes. Record merge SHA, evidence and checks
in the closing issue; practical acceptance remains separately required. No release
without practical acceptance of the exact package. Main push CI checks integration;
repair or revert regressions promptly.

Status comments are attestations, not cryptographic proof. GitHub check names can
be imitated by changed workflows. The maintainer personally inspects workflow and
executed-script diffs before merge and does not confuse a skipped job with the
explicit review status on the PR head.
