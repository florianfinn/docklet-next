# M8 storage and file workspace acceptance — 2026-10-23

[M8 closing PR #195](https://github.com/florianfinn/docklet-next/pull/195) merged
at `9e4cea9138bc3a03c8194e668a08ff19d848dc65` after three independent closing
review rounds (R1 and R2 fail, R3 pass). The accepted package is that main
commit, with green main CI. This records sandbox acceptance, not a product
release or production deployment.

Sandbox runs of candidate packages before the closing review found two defects
that synthetic tests had missed:

| Issue | PR | Defect found in the sandbox |
| --- | --- | --- |
| #191 | #192 | The internal volume helper counted as a volume consumer, so rename and delete stayed disabled after opening the Files tab |
| #183 | #195 | After a conflict reload, a clean open file kept a stale workspace revision and its next save reported a spurious conflict (fixed in closing round R3) |

## Accepted scope and method

Hub and agent images were built from `deploy/hub/Dockerfile` at the exact main
commit and deployed to the disposable sandbox hub, one Direct agent and one Edge
agent, after consistent data backups. The helper image was a local build of
`deploy/tools` from the same tree under the default reference; it is not yet
published (#196). A Playwright Chromium script drove the production UI on both
hosts, in German at 1440 px and in English at 390 px, and checked results
through the API:

1. Create a volume in the UI; open its detail page.
2. Create a file in the Files tab, edit it and save; the server holds the text.
3. Change the workspace from outside; creating a folder in the UI conflicts,
   the tree reloads, the external entry is kept and a retry succeeds.
4. Save a clean open file after that reload, and a second file after an own
   save; both succeed without a conflict.
5. Replace an open file outside the tab by delete and upload, then edit and
   save in the tab; the new content is kept and the edit applies to it.
6. Swap two files by renaming while one is open in a tab, then save the tab;
   the save is refused and the real content is kept.
7. Rename the volume (the page follows the new name) and delete it.
8. Open the prune dialog (named volumes are excluded by default), include named
   volumes, select exactly one prepared volume and delete; only that volume is
   removed and the result lists it.
9. Edit an additional stack file in the editor and Save & redeploy; the file is
   stored and the deploy run completes successfully.
10. The Backups page shows the neutral unavailable state (backups belong to M12).

All steps passed on both hosts in both locales. Steps 8 and 9 were rerun with
corrected script selectors after the first acceptance run failed on script
errors (a case-sensitive button name and a host-specific file name); the
product behaviour was unchanged between runs. HTTP errors observed were the
intended 409 conflicts of steps 3 and 6 and 404 reads after rename and delete.

## Limits and follow-ups

- The Docklet tools image is not yet published; its first publication and the
  digest pin need maintainer approval (#196).
- Browser consoles logged aborted activity reads during navigation; direct reads
  of the same resources succeed.
- The stack Deployments tab listed no deploy history after the successful
  redeploy; outside M8 scope, tracked in #198.
- Accepted follow-ups: #193 (a detached helper create can recreate a removed
  volume), #194 (volume usage returns 500 for a missing volume), #197
  (conflict-recovery polish).
- Not exercised: scoped non-administrator users, older agents, other browsers,
  very large volumes, bind-mount browsing beyond the neutral state.
