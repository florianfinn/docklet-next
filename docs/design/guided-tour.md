<!--
Source: docklet-next/issues/640
Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
Retrieved: 2026-11-26
License: Apache-2.0
Deviations: Original guided tour rationale.
-->

# Guided tour after first-run setup

Decision for #640 (maintainer, 2026-11-26): a short, optional walk through the most
important pages for administrators, offered when first-run setup finishes.

## Behaviour

- Finishing setup offers the tour once on the dashboard (Start tour / Not now).
  **Settings** › **Explanations** restarts it at any time.
- Nine steps cover the dashboard ring, the host filter, hosts, stacks, containers,
  security, backups, notifications, and search with activity.
- The tour never changes pages on its own. A step on another page highlights that
  page's navigation entry (on phones, first the menu button), and the step continues
  once the page opens. The user can open it through the link, through Next or with
  the arrow key; Next opens the page on request.
- The rest of the page is dimmed and blocked. While a navigation entry is
  highlighted, only that entry is clickable. Escape ends the tour.
- Each step names anchor candidates in order. A better candidate that renders later
  (a lazily loaded page, the opened phone menu) replaces a fallback. If no anchor is
  visible after a short wait, the card shows centred without a highlight.
- Only administrators get the tour. Simple users see neither the offer nor the start
  button.

## Rejected alternatives

- **Automatic page changes.** The maintainer wants users to learn the navigation, so
  the tour points at it instead of moving on by itself.
- **Remembering that the tour ran.** Persisting a flag would offer the tour to
  existing hubs after an update, but it adds a stored preference for little benefit.
  The tour stays session state, so the change needs no preference, API, OpenAPI or
  migration work.
- **A tour library.** The candidates with suitable licences still need custom anchor
  handling for lazy pages and the phone menu. A small helper on the existing button
  and catalogue primitives is easier to follow.

## Tests

`web/src/shared/tour/guided-tour.test.tsx` covers the offer, the navigation phase
through Next and through the link, page steps, Back, Escape, finishing, the phone
menu hint, the centred fallback, the simple-user exclusion and card placement. The
setup and help settings tests cover the offer after setup and the restart.
