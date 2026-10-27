# M10 images and updates acceptance — 2026-10-27

The accepted package is main `ecf29c1cbd3a7636347c4ff552a80bc63fa26589`, the
squash merge of the [M10 closing PR #251](https://github.com/florianfinn/docklet-next/pull/251).

One independent closing review round passed with no blockers. Three of its
non-blocking findings were fixed before the merge in #258, whose fix
verification passed. This file records sandbox acceptance. It is not a product
release or a production deployment.

## Integrated steps

Each step reached the feature branch through its own PR. Each PR had an
independent review by a different provider than the implementation.

| PR | Issue | Step | Review outcome |
| --- | --- | --- | --- |
| #234 | #225, #223 | Activity discovery without detail fan-out | Design R1–R2 failed, R3 passed; step review passed |
| #237 | #229 | Image and registry guards, contract v7 | R1 and R2 failed, R3 passed |
| #238 | #229 | Image and registry capabilities and typed errors | Passed |
| #239 | #230 | Images and registries web UI | R1 failed with 2 blockers, R2 passed |
| #240 | #229 | Stack image update and restore lifecycle | R1 failed, R2 passed, retention fix verified |
| #241 | #229 | Update policies, hub schedules, notifications | R1 failed with 3 blocker classes; fix verifications passed; blind control review passed |
| #243 | #229 | Update capabilities and restore-point protection | Design R1 and R2 failed, then a root-cause plan; step review passed |
| #244 | #231, #198 | Updates, restore points and policies web UI | Design R2 passed; step R1 failed with 1 blocker, fix verified |
| #247 | #229 | Image identity per store, new-stack policy read | Design R1 and R2 failed, then a root-cause plan; step R1 failed with 1 blocker, fix verified |
| #249 | #231 | Shorthand references, detail projection, dialog bounds | Fix verification passed |
| #258 | #229 | Remote lifecycle result validation, orphan-point access | Closing-review findings; fix verification passed |

## Defects found in the sandbox

Three candidate runs on the integrated feature head found eight product defects
that tests had missed. All were fixed before closing.

| Run | Defect | Fixed in |
| --- | --- | --- |
| R1 | Every update check failed with 503 under the containerd image store. The local image ID is the index or manifest digest there, not the config digest. | #247 |
| R1 | A never-configured stack returned an empty policy target identity, so the policy editor was unusable. | #247 |
| R1 | The mobile pull dialog overflowed at 390 px. | #247 |
| R2 | Shorthand image references (`busybox:1.35.0`) did not match canonical observations, so update candidates were dropped. | #249 |
| R2 | Stack detail lost the checked update state on reload. | #249 |
| R2 | The deploy-once guidance for baseline-free stacks was unreachable. | #249 |
| R2 | The German restore dialog overflowed at 390 px, and its action was unreachable. | #249 |
| Review | The step review of #247 found that platform variants (`arm64` vs `arm64/v8`) were compared unnormalised. | #247 |

R1 also found that an Edge agent stays offline after a frozen process. The
cause is pre-existing and is tracked in #245.

## Accepted scope and method

Hub and agent images were built at the exact main commit and deployed to the
disposable sandbox hub after data backups. The sandbox had one Direct agent and
one Edge agent on agent contract v7, both using the containerd image store.

Package checks:

- Migrations 006–009 applied at startup.
- The served bundle hash matched the local build.

Browser runs:

- Playwright Chromium drove the production UI in four combinations: German and
  English, 1440 px and 390 px.
- Runs were sequential, with HTTP/2 disabled for the gating matrix.
- One separate observation run used HTTP/2.

Scenarios exercised through the real UI:

- Pull, remove and prune of images.
- Registries with write-only credentials.
- Editor deploy with deployment history.
- Drift refusal and the baseline-free deploy-once guidance.
- Manual version update with a shorthand tag.
- Restore, including the restore-target and healthy-baseline protections.
- Delete of an unprotected restore point.
- Notify and apply policies with a scheduled auto-apply that created a
  restore point.
- Version hints.
- Reload and host switching.
- Dialog bounds at 390 px.
- A strict console and request gate.

After update and restore, Docker inspect confirmed that the running containers
used the expected local image IDs.

## Result

All 48 matrix cells passed on the exact package, including Direct desktop
restore and delete. One cell used a single harness correction. The run recorded
no `ERR_FAILED` and no HTTP 429 responses.

The acceptance run marked two extra checks as not passed. Both have the same
pre-existing cause, so they do not block M10:

- **Direct held-agent receipt budget.** The browser observed no activity
  response while the Direct agent was paused for about 40 s.
- **Busy orphan deletes after the hold.** Afterwards, orphan restore-point
  deletes on Direct returned `image.busy` until the agent was restarted.

The Direct agent log shows the cause at the time of the pause:
`Francis host failed: … host is not registered`. A pause of about 40 s expired
the actor host registration, and the agent then kept serving HTTP without its
background workers. This is the pre-existing defect #245. M10 did not change
that code, and the activity budget code is identical to the code whose held-agent
check passed in candidate runs R2 and R3:

| Run | Direct | Edge |
| --- | --- | --- |
| R2 | 10.6 s | 30.6 s |
| R3 | 10.6 s | 30.6 s |

## Limits

- Restore-point hold state is not refreshed when a run settles. The restore
  action stays disabled until the list is read again (#250).
- No live classic image-store daemon was available. Classic cases are derived
  from the pinned Moby v29.9.0 source.
- An auto-apply that turns out to be a no-op at execution time shows as a failed
  run with `job.skipped`.
- No notification channel was configured, so notification delivery was not
  exercised. Only the candidate state was observed.
- The gating browser ran with HTTP/2 disabled because of the shared front proxy
  (#211).

## Open follow-ups

#235, #236, #242, #245, #248, #250, #252, #253, #254, #255, #256, #257.
