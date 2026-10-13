# Provenance and inventories

Each adopted file retains existing legal notices and adds a short native comment:

```text
Source: owner/repository/path/to/file
Revision: <full 40-character Git SHA or 64-character SHA-256>
Retrieved: YYYY-MM-DD
License: <SPDX identifier or expression>
Deviations: <specific adaptations and their reason, or None>
```

Use repository-relative source names, never workstation paths. Markdown uses an
HTML comment; JavaScript/Go use comments; shell/YAML use #. The provenance guard
checks these fields at the start of files in the configured adopted prefixes or
exact file list. Missing/malformed headers fail. Register imported paths before
import and retain whole-prefix coverage as files arrive. A header describes
provenance, not an implementation history.

Formats without comments, such as .claude/settings.json, use an exact entry in
scripts/provenance.json with the same fields plus the current file SHA-256; content
changes require updating the entry and deviations. Byte-identical legal texts are
instead protected by the licence hash manifest. Source licences remain intact.

scripts/size-baseline.json maps adopted oversized files to reviewed line ceilings.
Own files have a hard 1,000-line limit without exemptions. A first adoption may
add an oversized file and its ceiling together only if the path is absent from
the comparison base, belongs to a configured adopted prefix or exact file entry,
has a valid provenance header in its first 40 lines, and the ceiling equals its
exact line count. An existing own file cannot acquire an adoption exemption.
This is the import path for oversized upstream files; no separate oversized base
or comparison override is needed. Review each imported file and ceiling together.

After adoption, ceilings may only shrink or disappear. A file may not exceed
its ceiling or its actual count in the comparison base, even under an older,
higher ceiling. Count LF, CRLF and CR as one line ending each; a terminal ending
does not add an empty line. CI uses the target base; local checks default to HEAD,
or an empty base for an unborn repository. Lower ceilings when shrinking files.

Generated exemptions are exact paths in the separate generated list. Each must
exist and match the short reviewed classifier in scripts/check-structure.mjs:
lockfile basenames pnpm-lock.yaml, package-lock.json, go.sum or yarn.lock, or files
under api/generated/ or web/src/generated/ with a whole-word generated marker
(case insensitive) in the first 4,096 bytes. Arbitrary paths, missing markers and
stale entries fail, even when added to the list in the same change. No directory
is exempt as a whole; adding a supported directory requires a guard-code change.
A marker alone is not proof of generation: review the generator and its output.

CODEOWNERS assigns guard code, tests, policy, licences and governance changes to
the maintainer. GitHub must require code-owner approval for this to enforce review.
Provenance, asset and licence policy edits remain review-controlled; their JSON
cannot prove upstream origin, redistribution rights or publication suitability.

## Dependency licence inventory format

Maintain a JSON inventory per ecosystem with schemaVersion, ecosystem (go or npm),
lockfiles (path plus SHA-256), target/build tags, tool/version/date and packages.
Each package records name, exact version or revision, source URL/path, scope
(runtime/build/test), direct/transitive, SPDX licence expression, selected alternative,
copyright, licence-file paths and hashes, modifications, corresponding source
reference, redistribution obligations and disposition (accepted/blocked).

Go covers backend, types, CLI, each go.getarcane.app module and all dependencies.
Inventory the actual shipped binary graph separately from test/build dependencies;
record platform and tags using go list -deps and licence tooling plus manual
resolution. npm covers the pinned web runtime graph and identifies development
packages separately. Record generated copies, fonts (including OFL-1.1), free
shadcn MIT notices and logo module hashes. Unknown/incompatible licences block release.

M1/M2 implement lockfile-to-inventory drift checks alongside the first respective
lockfiles. M0 has no Go/npm dependencies or product lockfiles and claims no inventory
completeness. See NOTICE for explicit pending MPL-2.0 and EDL-1.0 duties.

## Asset allowlist

scripts/assets.json is a list of objects with path, sha256, source, license and
purpose. Every binary or media file (including text SVG) must match an entry and
its SHA-256. Stale entries fail too. Before adding an entry, review licence and
publication suitability; licence/source text may refer to the provenance inventory.
Do not admit videos, music/audio, render caches or real-environment screenshots.
Only synthetic screenshots may be reviewed and allowlisted. M0 has no assets.

## Distribution hook points

scripts/license-policy.json records the immutable Arcane BSD text hash. M1/M2 must
add exact extracted build-artefact paths to artifactCopies and run the licence check
against each staging tree before packaging. A configured copy must exist and match;
no copies are configured while there are no artefacts. Binaries/images/installers
must ship the same notice under an appropriate licences directory. The UI must
expose third-party licence text. Release checks inspect extracted artefacts, not
only a source Dockerfile or an in-app link.
