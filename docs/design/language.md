# Repository and interface language

All identifiers, filenames, explanatory comments, documentation, issues, PRs and
commit/merge text are English. Original legal and provenance notices are preserved.
Collaboration with the maintainer may be German.

English is the UI base locale and catalogue type. German must be complete and is
independent of repository language. User-facing strings belong in feature/platform
catalogues, not component literals. Validate matching keys, ICU syntax and matching
placeholder names/types, including plural/select branches. Double literal ICU
apostrophes. Backend run messages use stable codes with parameters, not translated
prose; render them at the UI boundary. The hub time zone comes from `TZ` at
startup; a selectable zone follows in M18
([core adoption ledger](core-adoption.md#decision-ledger)).

German ASCII-transliteration and umlaut-spelling prose guards are deliberately
absent. Publication checks still reject malformed UTF-8, replacement characters
and double-encoded text in files, commits and PRs. M2 adds catalogue validation
with the imported UI; M0 does not claim to validate catalogues that do not exist.

Binding source: repository-rule-additions.md, SHA-256
`ac725a10a363d728f9a48492cfebac9b49dfda9f72f16ece4c6d1811c3a85419`.
