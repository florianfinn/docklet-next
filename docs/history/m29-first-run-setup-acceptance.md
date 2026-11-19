# M29 first-run setup acceptance — 2026-11-19

The accepted package is main `2fe0cbabaed58d9dbdd4f26a94ed2ea8f78858d9`, the
[M29 closing PR #523](https://github.com/florianfinn/docklet-next/pull/523). Its
tree equals the feature head `0f6e3640`, which passed the final sandbox delta run.
The sandbox kit built the packages locally from the exact source; no registry
image was used.

This file records sandbox acceptance. It is not a production deployment; the
release of this package will be recorded on #517.

## Integrated steps

Steps were pushed to the feature branch as commits, without step PRs, and CI ran
on every pushed head.
- The Claude main thread planned the milestone, wrote the design notes, built S1,
  S3, the S2 web part and S4, and made every correction after the reviews.
- A Codex worker built the S2 backend part; Claude reviewed it. Every other review
  was Codex.
- A bundled design review failed with eleven findings (pending gate on upgraded
  hubs, proxy guard by header presence, forced fallback, bridge addresses on
  inspect failure, palette mapping, acceptance evidence and others). The notes
  were revised; its verify left one point, settled by decision AUTO-P0 (container
  detection reports only `container` or `unknown`).

| Issue | Step | Review outcome |
| --- | --- | --- |
| #518 | S1 identity backend: setup code, claim and takeover, stored sign-in policy, `AUTH_MODE` override, runtime change, recovery | R1 failed with six blockers and two majors (proxy switch checked a header, not the caller; open streams survived a change; static key without grants after the claim; forced recovery sequence refused; blank username as HTTP 500; pending forwarding without its code; break-map gaps; release smoke without a claim). Verify 1 failed with two blockers (late policy binding, no real Direct/Edge transport test), verify 2 with one (forwarding lost the bound context). The closing review found the seams sound. |
| #518 | S3 web identity: claim page, gates, wizard steps, Settings › Sign-in | Covered by the S1 rounds and the closing review. |
| #519 #520 | S2 container-aware discovery: hub container, published ports, macvlan and host interfaces, `kind` and sources | Backend passed with two non-blockers (strict check on configured names, request scheme behind TLS proxies), both fixed. Web failed with one blocker (onboarding ignored the saved IP address) and one non-blocker (overlay shown as bridge), both fixed. |
| #520 #521 #522 | S4 IP address and Domain fields, link choice, nebula default, TZ link, module cards | R1 failed with one blocker (a cleared domain was refilled by late discovery), fixed. |
| #517 | Closing integration (#523) | The closing review failed with three blockers in the wizard initialization: a typed IP blocked domain prefill; edits and module states were lost around the saved-settings reads; saved values were dropped when both addresses were blank. Decision AUTO-4 fixed them (wait for both reads, start from saved values). The verify passed with one non-blocker (lab-only draft reset), tracked as a follow-up. The acceptance fix for PD4 passed review with no findings. |

## Findings during acceptance

An early kit run on S3 (`841edee4`) prepared the fixtures and passed its smoke
and regression checks. The candidate run and the deltas found:
- **PD1, A7c.** A host-network hub could not identify its own container under a
  custom Docker data root and cgroup v2. Detection now also reads the container
  ID from the mount and cgroup paths.
- **PD2, A8.** The domain was prefilled only together with a usable IP address.
  Each field now fills on its own.
- **PD3, A10.** A shared field rule stretched the "TZ verwenden" link to the full
  width; it now spans its text.
- **PD4, A5 (delta 2, PD1 in that run's report).** After a successful claim, "Passwort eingeben" appeared
  briefly before the wizard opened, because the cleared form kept its validation
  state. It now resets with the fields.

## Accepted scope and method

The sandbox had the hub-local host H, one Direct agent D and one Edge agent E,
plus disposable discovery fixtures (bridge, host network, macvlan) and a frozen
1.0.0 database for the upgrade cell. Product checks used real browser gestures,
with API reads as witnesses; visible UI errors count as failures. Some setup
writes used the real hub APIs: A2 completed the configuration through the partial
settings API after a UI claim, and A11 registered the agents through the
registration API.

| Cell | Behaviour |
| --- | --- |
| A1 | Fresh hub, local sign-in: setup code in the log, wrong code refused, claim, five wizard steps, Finish to the dashboard, sign-in with the new administrator |
| A2, A3 | Fresh hub with no sign-in; with reverse-proxy sign-in, where a direct claim is refused with `auth.proxy-unverified` |
| A4 | `AUTH_MODE` set: mode locked in setup and Settings, change refused with `auth.mode-forced` |
| A5 | Upgrade from 1.0.0: setup opens, the old `arcane` login is refused, the administrator keeps its ID, the wizard starts from saved values, host and stack stay online |
| A6 | Mode changes local, none and local again without restart; sessions revoked; proxy refused from a direct browser |
| A7a to A7d | Discovery on bridge with a published port, loopback binding, host network and macvlan |
| A8 | IP address and Domain prefill, link choice, saved values reload, a cleared domain stays empty |
| A9, A10 | Nebula default palette; TZ link width, module cards, five steps |
| A11 | Direct and Edge agents stay online across mode changes; stack actions work through each |
| REG | Standard regression |

Exclusion agreed at kickoff: external LAN reachability of the offered addresses
in A7c and A7d, because the sandbox has no second LAN host; candidate correctness
was checked.

## Results

| Package | Result | Notes |
| --- | --- | --- |
| `c020e7af` (candidate) | Failed, 12/15 | A7c (PD1), A8 (PD2), A10 (PD3) |
| `9ee12a69` (delta 1) | **Accepted**, 15/15 | A7a to A7d, A8, A10 and an A1 subset rerun |
| `6ae5d39f` (delta 2) | Failed, 14/15 | After the closing review fixes: A1 subset and A8 passed; A5 failed on PD4 while every preservation check passed |
| `0f6e3640` (delta 3) | **Accepted**, 15/15 | A5 in full and the A1 subset |
| `2fe0cbab` (exact main) | **Accepted**, 15/15 | Tree equals `0f6e3640`; A1 subset and REG on H rerun; served assets equal the built image |

Unchanged passing cells carry their evidence forward, so each denominator counts
all 15 cells, not 15 newly executed ones.

## Limits

- Reachability of offered addresses from other LAN hosts was not tested.
- Chromium only. The upgrade was exercised from one 1.0.0 database.
- D, E and the discovery fixtures stayed on earlier M29 heads in the later deltas;
  their cells carried over.

## Open follow-ups

#524 (wizard after a draft reset on a mounted connected page, lab only), #525
(deterministic witness for the token cache race).
