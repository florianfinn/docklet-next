# M7 stack authoring acceptance — 2026-10-21

[M7 closing PR #141](https://github.com/florianfinn/docklet-next/pull/141) merged
at `3ccd9e86572ccd7150b2aaf5bf04c555e98a284c` after four independent review
rounds (R1–R3 fail, R4 pass). Practical acceptance in the sandbox found four
defects in the merged package. Each was fixed on main through its own PR and an
independent review by a different provider:

| Issue | PR | Defect found in the sandbox |
| --- | --- | --- |
| #146 | #147 | Accepted remote runs stayed running and kept their stack scope until a reader refreshed them |
| #150 | #151 | The production editor of an existing stack opened without its stored files |
| #152 | #154 | Label changes on Edge hosts were refused by the tunnel command table |
| #155 | #156 | A refresh on opening the run dialog could fence a deploy before Execute reached an Edge agent |

PR #160 raised the CI budget of the full check stages (#159). The accepted
package is main `1aca4a1121b9774b20cda0555dcd676611501eaa`, with green main CI.
This records sandbox acceptance, not a product release or production deployment.

## Accepted scope and method

Hub and agent images were built from `deploy/hub/Dockerfile` at the exact main
commit and deployed to the disposable sandbox hub, one Direct agent and one Edge
agent. Consistent data backups were taken first, and `/health` was checked.
A Playwright Chromium script drove the production UI on both hosts, in German at
1440 px and in English at 390 px. It checked results through the API and the
Docker daemon of each host:

1. Create a stack in the editor and deploy it; the run settles successfully
   without an explicit refresh, and the container runs.
2. Edit compose and `.env`, add a label, and Save & redeploy; the new
   environment reaches the container and the label persists.
3. Reopen the stack; the editor shows the stored files.
4. Convert a `docker run` command with a named volume on the hub, add it and
   save; the stored compose keeps the existing service and gains the external
   volume declaration.
5. A second runtime command during a running deploy returns 409 `run.busy`.
6. A deploy with a missing image settles failed.
7. Invalid compose disables deploy.
8. The temporary project is destroyed.

All 26 checks per run passed in both runs (exit 0). The only console error was
the expected initial unauthenticated identity read. No layout overflow occurred.

## Limits and follow-ups

- Failed runs show an English hub reason in the German view, an incomplete
  headline and no pull error output (#157).
- In hub conversion mode the dialog shows an empty notes heading.
- Accepted follow-ups: #142 (Unicode label normalization), #148 and #153
  (flaky tests), #149 and #158 (runtime refresh edge cases), #159 (CI duration).
- Not exercised: scoped non-administrator users, older agents, other browsers
  and real production hosts. Detailed evidence, screenshots and logs stay private.
