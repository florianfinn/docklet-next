# M22 publication readiness acceptance — 2026-11-10

The accepted package is main `ebfa0f9d759998e3c315e322d64d6778b8efcdc4`, the
squash merge of the [M22 closing PR #459](https://github.com/florianfinn/docklet-next/pull/459).
Its tree equals the merged feature head `7b1cd04a`. The acceptance runs used the
`1.0.0-rc.N` dry-run packages that the release workflow built on the pull request.

This file records sandbox acceptance. It is not a product release, a published
image or a production deployment. By maintainer decision the first publication
waits until the remaining open issues are worked.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head. The Claude main thread wrote the design notes and
implemented every step and fix; Codex Sol reviewed them.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #420 | S1 provenance headers first in ten web files; `provenance-header-order` guard | Checked in the closing review: 1 blocker (the guard searched only the first 40 lines, so a sidecar-backed file could put code above a later header). Fixed in `7b1cd04a`; the verify passed. |
| #448 | S1 publication performance guard re-measures a noisy sample, at most three attempts | Checked in the closing review without findings. 20 consecutive runs under parallel load passed. |
| #450 | S2 canonical IPv6 comparison; unused allowlist entries fail only in the current policy | R1 passed without findings. A full history scan of 96 commits passed. |
| #446 | S3 CA source archive and licence texts in the release package, resumed uploads behind redirecting blob storage, rendered pin guard | R1 failed with 1 blocker (the variable named `_IMAGE`) and 1 non-blocker. The first verify failed with 1 blocker (an image fallback nested in another default); after this second failing round in one class the guard was rebuilt as an interpolation scanner. The second verify passed; its non-blocker was fixed. |
| #454 | Closing integration (#459) | The closing review failed with the #420 blocker above; the closing verify passed. |

The design notes had one bundled design review (3 blockers, 3 non-blockers) and
a delta verify (1 blocker that applied only to an alternative not chosen). The
main finding was that the CA source archive also contains MIT and curl licensed
utilities, so the release attaches their licence texts too.

## Defects found before acceptance

The sandbox runs found no product defect. Two runner faults were fixed inside
their runs: an image identity check that assumed the classic image store, and a
render check without a placeholder for the required encryption key.

## Accepted scope and method

The candidate packages were the release workflow's own `release-oci` and
`release-package` artifacts of the pull request head, downloaded unchanged. The
OCI layouts were uploaded to the sandbox registry with the repository's uploader
and read back by digest; nothing was rebuilt. The sandbox had the hub-local host,
one Direct agent and one Edge agent.

| Cell | Behaviour |
| --- | --- |
| RP1 | Release dry run green; the package contains the CA source archive with the MPL-2.0, MIT and curl texts and the CA notice; every file is in `SHA256SUMS`; the archive SHA-512 matches the notice and `deploy/release/ca-bundle.json` |
| RP2 | Every image default in the rendered Compose file is the pinned hub or agent reference; the operator overrides stay in the env example |
| RP3 | An upload interrupted after a committed blob resumes through a loopback proxy that answers stored-blob HEAD requests with 307; only the remaining blobs are uploaded and the redirect target receives no request |
| SM1 | Package-aware upgrade of all three hosts from `1.0.0-rc.2` (the M21 package) to the candidate, with backup, readiness, smoke and image identity checks |
| LIC1 | The installed hub and agent images ship the new CA notice |
| REG | Regression walk on the hub-local host |

The agreed exclusions: the publication itself (tag, `release` environment,
registry upload and release creation) is not exercised, and the loopback proxy
stands in for a registry with redirecting storage. The request budget was
advisory.

## Result

| Package | Result | Notes |
| --- | --- | --- |
| `829f605b` (early smoke, `1.0.0-rc.30`) | Pass, 6/6 steps | Proved the unchanged-artifact upgrade procedure on all three hosts |
| `6c4327a5` (candidate, `1.0.0-rc.31`) | **Accepted**, 6 PASS / 0 FAIL | Full matrix |
| `5e064d5b` (delta 2) | **Accepted**, 6 PASS / 0 FAIL | RP1 and RP2 rerun after the pin-guard fixes; the eight unversioned package files were byte-identical to the candidate; the other cells were carried |
| `ebfa0f9d` (exact main) | **Accepted**, 6 PASS / 0 FAIL | Tree equals `7b1cd04a`; the only change after `5e064d5b` is the header-order guard, so all six cells were carried; main push CI run 42919374201 green including the publication history scan; fresh sign-in and Dashboard pass; all three hosts online |

## Limits

- Publication was not exercised; the GitHub `release` environment does not exist
  yet and must be configured before the first tag.
- The redirect behaviour of a real registry was replaced by a loopback proxy.
- Request-only events without a visible error remained: the anonymous start-up
  check after sign-in and aborted reads during navigation. They are advisory and
  tracked in #425.
- The interpolation scanner refuses `$$`-escaped expressions in the rendered
  Compose file, which is stricter than Compose.
- Direct and Edge agents ran on separate Docker daemons on one shared kernel.
  Chromium only.

## Open follow-ups

None from M22. The remaining work is planned in milestones M23 to M26.
