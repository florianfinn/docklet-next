<!--
Source: docklet-next/issues/339
Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
Retrieved: 2026-11-03
License: Apache-2.0
Deviations: Original personal preference storage rationale.
-->

# Personal preferences on the user profile

Decision for #339 (M17, maintainer kickoff 2026-11-03), implementing core adoption R7:
personal appearance, views and dashboard layout live in the hub database on the user
profile, or as hub preferences without login. Browser storage becomes a cache.

## 1. Storage and routes

- `users.ui_preferences` holds one JSON document `{revision, sections}`; the older
  `preferences` field and the language field stay as they are.
- Sections and their values:

  | Section | Value |
  | --- | --- |
  | `appearance` | design tokens, without the design lab's comparison slots |
  | `uiStyle`, `visualStyle` | interface and visual style settings |
  | `views` | overview modes and layouts, host filter, workspace and sidebar host list |
  | `hidden` | stacks and containers hidden from overviews |
  | `help` | help mode |
  | `dashboard` | widget order, hidden widgets and sizes |
  | `fleetView` | fleet ring view |
  | `favorite` | `{value}`, the saved design code or `null` |
  | `sidebar` | `{open}` |

- `GET /auth/me/ui-preferences` returns the document; `PUT /auth/me/ui-preferences/{section}`
  replaces one section. Both act on the signed-in user only. A section must be an object
  of at most 16 KiB, the document at most 64 KiB. Errors: `preferences.no-user` (host and
  agent tokens), `preferences.unknown-section`, `preferences.invalid`,
  `preferences.too-large`. A stored document that cannot be read is served as empty.
- Without login the document belongs to the global administrator, which makes it the hub
  preference. Proxy accounts that map to that administrator share it.

## 2. Web synchronisation

- Every section has a decoder that checks field types and values and falls back to
  defaults field by field, for server data and for cached data alike.
- After sign-in the web loads the document and applies it. Changes are sent per section
  after one second; edits made while loading are kept; an older response never undoes a
  newer edit; failures are retried with the next change.
- First load after the upgrade: the browser claims its existing values for the signed-in
  user before uploading anything, then uploads each section the server does not have yet.
  An interrupted upload resumes for the same user only; another user on the same browser
  never receives those values, and sections already on the server are never overwritten.
- On sign-out or a change of user, pending writes and late responses of the previous user
  are dropped and the stores start from the new user's document.
- Several open tabs: the last write of a section wins; other tabs see it after reloading.
- Design lab comparison slots and the demo simulation stay in the browser. Design codes
  still export and import; applying one writes through the stores and is synchronised.

## 3. Break map

| ID | Row | Test |
| --- | --- | --- |
| PF1 | user change and late responses | sign-out during a pending or running write |
| PF2 | first-load migration | interrupted upload, other user, resume for the owner |
| PF3 | failed load or write | cache kept, no migration, newer edits kept |
| PF4 | malformed documents | server limits and web decoders up to rendering |
| PF5 | sign-in modes | no login, proxy, local, simple user, API key, host token |
| PF6 | access | only the own document |
| PF7 | migration | existing users get an empty document |
| PF8 | error codes | contract and response mapping |
| PF9 | clearing the favourite | clear, reload, fresh browser |
