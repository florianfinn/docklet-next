# Frontend consolidation

Adopt the approved React/shadcn interface in full. This consolidates the confirmed
prototype decision (R3); it supersedes Angular/ZardUI wording in the earlier
2026-10-10 decisions and design concept. It is not a new framework evaluation.

The frozen remediation snapshot of 2026-10-12 contains 592 src/ files with SHA-256
aggregate `d75474089b87ab9f77c606cc7aa696d05157d8e9cd902f1ed5a0f2b904722a39`.
Compute SHA-256 over the byte-sorted list `<file SHA-256>  <path>\n`, with paths
relative to the snapshot root and including the src/ prefix. Compare this value
before and after the unchanged M2 snapshot copy, then document subsequent changes.
The reference dist/ aggregate is
`7ec6917f30ae83b9830920fcbaba82bf7c9c2080429fbb347e58334299f1d556`.
These are reference evidence, not claims that product code is already present.

Preserve approved routes, pages, components, tokens, palettes, forms, wizards,
domain logic, types, translations, preferences and tests. Do not redesign or rename
components for taste. Later approved behaviour takes precedence over early wording:
whole-grid-cell widget sizes and an optional Focus dialog for important actions
remain part of the reference. Remove wording implying an Arcane palette is a
Docklet default (R9); check palette naming for endorsement implications.

M2 first imports the unchanged snapshot with provenance. It then separates demo
transport and the design-lab panel behind a development/test build boundary,
backed by a production bundle guard. No route, setting or query enables them in
production. User colours, views and design-code import/export move into product
preference/theme modules. Keep typed replaceable DockletAdapter interfaces;
HTTP/WebSocket integration follows in M3. Simulation flags stay in demo options.

English becomes the catalogue base type; German remains complete. Use normal key,
ICU and placeholder checks after the one-time snapshot hash proof. Prepare the hub
timezone instead of a hardcoded zone. Exercise complete workflows and loading,
empty, error and success states at 320, 390 and 1600 pixels, light/dark, DE/EN.
The prototype's measured tests and browser checks are not live product acceptance.

Source basis design-concept.md SHA-256:
`464f467cf47ffe130ecaca0f0bf6434db2d1420c15fa43ae860b2baee5ff7d8c`.
Source basis repository-rule-additions.md SHA-256:
`ac725a10a363d728f9a48492cfebac9b49dfda9f72f16ece4c6d1811c3a85419`.
