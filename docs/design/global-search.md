<!--
Source: docklet-next/issues/356
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original global search and command palette rationale.
-->

# Global search (command palette)

Decision for #356 (M19, maintainer kickoff 2026-11-06). Docklet is a homelab tool with normally one
administrator; simple users are an edge case and see only what their shell mounts.

## Accepted decisions (do not reopen)

- Quick actions are navigation and create only: New stack, Add host, Check all updates, Activity
  centre, Toggle theme, Sign out, each only when allowed. No lifecycle actions on resources.
- Simple users get no host results. They get Dashboard, Stacks, Profile, their assigned stacks,
  Toggle theme and Sign out; a host name stays searchable as a stack's host chip.
- The Arcane settings search is removed.

## Starting point (main b5068aff)

- No palette and no Ctrl/⌘+K handler. `cmdk` and the shadcn `command.tsx` primitive are installed but
  unused (`web/src/shared/ui/command.tsx`). The click dummy has no palette; the release plan adds it,
  so its UX is defined here.
- List data comes from existing query hooks (`web/src/data/queries.ts`): hosts, stacks, containers,
  volumes, images, settings; networks through topology. For simple users the hub filters hosts and
  stacks ([resource rights](resource-rights.md)); container, image, volume and network lists are
  admin-only. Only the containers hook disables itself for users (`useIsUser()`); images and topology
  rely on the shell not mounting them.
- Arcane settings search: `search-settings` and `get-settings-categories`
  (`settings/module.go:93-104`, `handler.go:435-438`, `service.go:416`) and `search-customize` and
  `get-customize-categories` (`internal/search/module.go:55-67`), all Admin, with no consumer in web
  or tests.

## Design

- Open with Ctrl+K / ⌘+K anywhere, also inside inputs, and with a search icon button in the header
  next to the user menu; Esc closes. Inside the code editor (`data-slot=code-editor`) the shortcut is
  ignored and the editor keeps its keymap. `CommandDialog` from the shadcn primitive, accessible name
  "Search"; the shortcut hint uses `kbd` and shows ⌘ on Apple platforms.
- The palette builds its entries from the route set the current shell mounts.
  - Administrators, in this order: Pages (main navigation), Settings (every settings section from
    `SettingsPage.tsx` GROUPS/IDS with catalogue label and keywords), Hosts, Stacks, Containers,
    Images, Volumes, Networks. Each result shows name and host chip; selecting navigates.
  - Simple users (`UserShell` allows `/`, `/dashboard`, `/stacks`, `/stacks/:id`, `/profile`): Pages
    Dashboard, Stacks, Profile; the Stacks group (assigned stacks); quick actions Toggle theme and
    Sign out. No Hosts group, no settings sections, no New stack, no Activity centre.
- Quick actions are shown when the query is empty or matches their label. Each administrator action
  is gated by its own capability (New stack: create-project; Add host and Check all updates: admin).
- Data: the same hooks as the lists, called while the palette is open (cached data shows at once;
  missing lists load with a per-group spinner). Admin group consumers are mounted only inside an
  admin-only component, so a user palette never instantiates those queries.
- Matching is client-side over name, host name and labels (cmdk filter over all candidates); at most
  8 results per group after matching, and "Show all in {list}" opens the list with the query as filter
  where the list supports a query parameter.
- The palette reads live query data and closes on an identity change.
- Removal of the Arcane settings search, complete: the four operations above, their DI wiring, rules
  (`access/rules.go`), route entries (`access/routes.go`), `api/removed-operations.json` entries and
  generated types; `SettingsSearchService` and the `internal/search` package go. The two
  category-policy assertions in `pkg/authz/access_policy_test.go:135,143` read the retained settings
  metadata source (the settings model categories) or are retired with a one-line reason if their
  surface is gone; no dangling service remains.
- EN/DE catalogue for all labels.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Simple user | only pages, stacks and allowed actions; no admin-only query sent | vitest with a user principal counting adapter calls to admin-only lists = 0; red proof: render groups for users → red |
| User destinations | every user entry navigates to a mounted route | vitest under the user shell selecting every entry; red proof: expose Hosts or New stack for users → red |
| Keyboard | Ctrl+K and ⌘+K open, Esc closes, arrows and Enter select | happy-dom keyboard test + one real Chromium check in the candidate run |
| Code editor | shortcut ignored inside the editor | keyboard test inside `data-slot=code-editor` |
| Stale results | live data; closes on identity change | test with the `identityLifetime` signal |
| Removed module | four operations gone from every mode's route set; rules and removed-operations agree; rule-completeness test still passes | route-set export test per mode; red proof: keep `get-settings-categories` or its rule → red; backend build + `TestM15RuleCompleteness` + removed-operations check |

## Contract

- OpenAPI loses four Arcane operations: `search-settings`, `get-settings-categories`,
  `search-customize`, `get-customize-categories` (listed in `api/removed-operations.json`).
- No migration, no agent contract change, no new route or code.
