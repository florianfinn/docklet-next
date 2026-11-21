<!--
Source: docklet-next/issues/562
Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
Retrieved: 2026-11-21
License: Apache-2.0
Deviations: Original M35 design summary.
-->

# Simpler everyday use (M35)

M35 removes wrong states, unneeded confirmations and excess text found in a
review of the running product, and makes the resource lists usable on phones.
The rule throughout: a hint where something matters, a block only where an
action cannot succeed or would lose data, one short sentence per hint.

## Stack update feedback (#555)

- **External drift status.** When a recorded env file changes outside Docklet,
  the baseline status carries the current saved and observed revisions,
  `available=false` and `update.config-drift`. Unreadable current inputs answer
  with the declared `update.snapshot-unavailable` failure; the specific reason
  stays on the stack itself.
- **Older agents.** Agents before M35 answered drift without revisions. The hub
  and the web accept an unavailable status with a known reason
  (`update.config-drift`, `update.baseline-unavailable`) and empty or missing
  revisions for display only. Mutations still need a 64-hex current revision: install is
  barred by the unavailable status, restore offers no consent and sends nothing
  until the revision is readable.
- **After submit.** The update dialog closes once the action is admitted; the
  run window or toast, as the presentation setting chooses, shows progress and
  the result.
- **Menu check.** "Check for updates" in the stack list runs the check in place.
- **Restore consent** names the replacement, not the revision hash; it is still
  bound to the full current revision and is cleared when that revision changes or
  cannot be read.
- **Run output.** Compose progress is stored as `{"log": …}` frames, unchanged.
  One presentation step shows a single-key string `log` frame as its line,
  decoded once, for display, folding, copy and download; every other frame is
  shown verbatim. Old runs read the same way.
- **Run details** name the result once and show unknown step progress as "–".

## Contract

No route, code, field or migration. OpenAPI and the agent contract stay
unchanged; the baseline validation only widens for unavailable statuses.
