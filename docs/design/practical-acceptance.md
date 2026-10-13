# Practical acceptance in a sandbox

Practical acceptance demonstrates a specified product revision and package against
real dependencies in a maintainer-approved isolated sandbox. Unit tests remain
independent of real services, Docker sockets and running agents. M0 establishes
this procedure only and deploys nothing.

Before a run, create an acceptance issue stating goal, exact commit/package and
image digests, prerequisites, permitted systems, module owner, test steps, expected
results, cleanup and recovery. Get explicit approval for the particular deployment;
a subagent must not alter a running system. Use origin/main for deployment.
Reserve test resources according to the sandbox owner's rules. Use synthetic data
and separate credentials; never reuse operating data or production secrets.

Back up the database before schema changes and record the backup verification
privately. Verify startup migrations, /health, served bundle and the tested feature
against the real core. Check error and recovery paths, agent/protocol compatibility,
restart persistence and installation instructions as applicable. A browser demo or
local unit test cannot establish these results.

For UI acceptance exercise 320/390/1600 pixel widths, light/dark and English/German,
including loading, empty, error and success states. Review screenshots for
publishability; only allowlisted synthetic evidence belongs in the public repo.
Public evidence identifies the revision, measurements, steps, results and limits
without hostnames, addresses, secrets or private links. Keep operational logs private.

Record cleanup, remaining risks and follow-ups. Link acceptance evidence, independent
review, CI and merge SHA from the closing issue. A release is accepted only after
all required features and the exact release package pass; source tests alone do
not authorise publication. Failures block release until fixed and rechecked.

For repository acceptance, verify with a test PR that the first pull_request_target
run creates the required review commit status before enabling the ruleset. The
OWNER comment attests to review; it cannot prove provider independence. Record
manual publication review of allowlisted binaries, leading-zero address notation
and mixed-script homograph emails in the acceptance evidence.

Enable required code-owner review for all governance paths in .github/CODEOWNERS,
including guard code, tests, policies, licences, hooks and design documents.
Record the accepted root SHA and verify that obsolete local roots are unreachable
and pruned before first publication. Rewriting the private root needs renewed
review of the resulting SHA; earlier review does not attest to changed content.
