<!--
Source: docklet-hub/docs/design/publication-policy.md
Revision: a4484c910bb5d6a8f8df97b4f23f4ee5e6359081
Retrieved: 2026-10-13
License: Apache-2.0
Deviations: English rewrite; no inherited operational networks; exact fixtures, IPv6 and local denylist.
-->

# Publication policy

All tracked data and Git metadata must be publishable. Do not include real
credentials, private email, hosts, domains, network plans, addresses, routes or
workstation paths. Reserved examples and exact synthetic parser fixtures are
permitted; no test directory is exempt. No prior repository history is imported.

The publication check reads staged blobs for pre-commit, tracked working files
for local checks, and commit trees/messages/identities for history/pre-push. PR
CI scans the actual incoming head history while checking the merged working tree
separately; it does not judge GitHub's synthetic merge message. It checks
filenames as well as contents. Findings contain only path and category, never
the matching text; unsafe filenames are replaced by a fixed label. Read and
configuration errors also fail without echoing untrusted content.

Checks cover credential patterns and literal secret assignments, non-example
email, non-example IPv4/IPv6 addresses, private host suffixes, personal absolute
paths, runtime env files/private artefacts, AI co-authors, session links,
invalid UTF-8, U+FFFD, unexpected control characters and double-encoded UTF-8.
Zero-width and bidi controls U+200B-200F, U+202A-202E, U+2060-2069 and U+FEFF
are rejected along with U+00AD, U+034F, U+061C, U+115F/1160, U+180E,
U+2028/2029, U+3164 and U+FFA0 in content, filenames and Git messages. Tests
construct these characters from escaped literals or code points; no raw
control-character exception is configured. Loopback addresses may be used only
for synthetic local test listeners. RFC 5737 and 2001:db8::/32 are the network
examples. Human Git identities use users.noreply.github.com; noreply@github.com
is allowed for GitHub commits. Reserved example-domain emails are allowed in
content, never as commit identities. No address exception permits an AI identity
or AI co-author.

## Literal-secret decision table

Names are case insensitive: password, passwd, secret, token, passphrase,
api_key/apikey, access_key/accessKey and private_key (also hyphens) match within
names. Credentials matches a word suffix; pwd is a whole name or underscore/
hyphen-separated suffix. OLDPWD is not a secret name. For credentials, pwd and
sensitive names ending in _file, -file, _path or -path, a value starting with /,
./, ../ or a drive prefix denotes a path. Other categories still check that path
for private workstation information.

The scanner classifies syntax before judging the value. Source extensions are
JS/TS (including JSX/TSX, CJS/MJS), Go, Rust, Java, C#, Python, Ruby, PHP, Vue
and Svelte. Quotes, raw/template strings and comments establish embedded text
contexts; keywords never turn an env or configuration file into source code.
Each row has rejected/accepted twins in publication-decisions.test.mjs,
publication-final-round.test.mjs or publication-r3.test.mjs.

| Row | Context | Finding | Non-finding |
| --- | --- | --- | --- |
| D1 | Untyped JS/TS assignment | Non-placeholder quoted value | Identifier |
| D2 | TS/Python typed assignment | Quoted value after type and equals | Environment/member expression |
| D3 | Go short, var or typed var declaration | Quoted literal | Header call or identifier |
| D4 | Env/shell equals assignment | Bare or quoted literal | Placeholder/substitution |
| D5 | Double-dash flag, space or equals | Bare or quoted literal | Placeholder |
| D6 | JSON/YAML scalar | Bare or quoted literal | Placeholder |
| D7 | Upper/lower key=value inside a source string | Bare literal | Placeholder |
| D8 | URL query parameter inside a string | Literal value | Placeholder |
| D9 | Source comment, equals or colon | Literal value | Placeholder |
| D10 | Multiline raw/template text | Embedded literal assignment | Placeholder |
| D11 | Shell default with - or :- | Non-placeholder default | Empty/placeholder default |
| D12 | YAML continuation or block scalar | Non-placeholder scalar | Placeholder scalar |
| D13 | YAML nested value | Scalar literal | Mapping or list structure |
| D14 | Fetch credentials option | Other literal | include, same-origin, omit |
| D15 | PWD or credentials location | Non-path literal | Path |
| D16 | Source right-hand side | Direct quoted value | Identifier, call, member access |
| D17 | Type annotation | Annotation followed by literal assignment | Annotation without value |
| D18 | Source equals operator | Literal assignment | Comparison |
| D19 | Shell/compose reference | Literal value or default; backticks outside shell files | $VAR, $(command); backtick command only in .sh/.bash/.zsh files and Dockerfiles |
| D20 | Workflow secret/permission mode | Other literal | Inherited secrets; read/write/none permissions |
| D21 | Human message string | Embedded key=value, leading key: value or Default key: value, including after escaped line endings | Other prose colons, format verb, closing quote |
| D22 | Python prefix or Rust reference type | Quoted prefixed/typed value | Placeholder or identifier |
| D23 | Source value on the next line | Quoted literal | Placeholder or identifier |
| D24 | Set/setItem method call | Sensitive literal key and literal value | Non-sensitive key, placeholder or expression |
| D25 | Adjacent Key/Value or name/value fields | Sensitive literal key and literal value | Non-sensitive key, placeholder or expression |

Every nonempty direct literal is rejected, including short values. Placeholders
are empty strings, changeme, example, test-only (case insensitive), all-asterisk
masks, angle-bracket placeholders, simple environment substitutions and GitHub
expression substitutions. Shell -, +, ?, = operators, with or without a colon,
are accepted only with empty or placeholder operands. A literal fallback is a
finding. Suffixes outside a substitution are not placeholders.

This is lexical recognition, not a language parser. Only the listed setter calls
and immediately adjacent named-value fields are recognized; reordered fields,
indirect names and other calls with literal arguments need manual review.
Expression fallbacks and constructed strings also need review,
including byte conversions, environment reads followed by an OR literal, cmp.Or
and t.Setenv. PHP/Ruby arrow pairs are outside the assignment grammar.
TS/Python/Rust annotations are recognized up to 256 characters of simple type syntax; complex
declarations, escaped/encoded secret names and template interpolation need
semantic review. Quoted public labels and translations under secret-like names
remain findings; use the exact reviewed procedure below instead of broad label
heuristics.

## Reviewed imported findings

scripts/publication-allowlist.json contains exact objects with path, category,
lineSha256, reason and an optional count. The previous sha256 spelling remains
accepted instead of lineSha256; supplying both is invalid. Count defaults to 1
and must be a positive safe integer. It is the exact number of occurrences of
that path/category/line-hash key, including multiple matches on one line. Fewer
actual occurrences fail as stale, including zero; extra occurrences remain
findings. No unused allowance survives a scan. Duplicate keys are rejected even
with different counts, reasons or digest spellings. Duplicate JSON object fields
also fail, including escaped spellings of the same key. Reasons must have at
least 12 trimmed characters and cannot start with TODO, TBD, FIXME or reason
(case insensitive), or contain angle-bracket placeholders. Invalid categories
and directory/glob paths also fail.

The digest is SHA-256 of the complete UTF-8 matched line without its line
ending; CRLF and CR normalize to LF. Indentation and trailing spaces remain
significant. For a multiline scalar, the matched line is the scalar value line.
Changing a line or moving its file invalidates the exception. Other categories
on the same line remain blocked too. Operator denylist, path, identity and
structural findings cannot be exempted here. The only eligible categories are
literal-secret, credential-pattern, non-example-email, network-address,
workstation-path and private-hostname. AI co-authors, session links, corrupted
or invisible text and invalid UTF-8 are never exemptable.

Before an import, prepare a temporary repository copy with the destination paths
and stage new files so the scanner can see them. From that repository root run:

```sh
node scripts/publication-allowlist.mjs --propose backend/ types/ cli/ web/src/
```

The command prints a JSON array with path, category, lineSha256, count and
reason "TODO" for all current tracked working-file findings in those scopes,
including findings already covered by a policy. Prefixes select an exact file
or a directory boundary, not similarly named siblings; overlapping prefixes do
not multiply counts. Untracked files are excluded. Only eligible content
categories are proposed; binary content is skipped. Unsafe filenames, invalid
UTF-8, unsupported entries and read/configuration errors fail without partial
output. Matched values and source lines are never printed. The command does not
write or approve a policy. Other guards and operator-denylist findings still
require separate remediation.

Inspect each finding privately and confirm that it is public upstream code, a
public label or synthetic data. Replace every TODO with a specific public
reason, review the exact count, and add the reviewed entries to the policy. Do
not record matched values in reports or the allowlist. Review each entry with
its code and provenance, including licence and publication safety. Do not exempt
actual secrets. Remove or adjust entries when the code stops producing the
recorded count. The index uses the staged policy; each historical tree uses its
own policy. A later exception cannot excuse earlier unsafe history. The initial
list is empty; imports add only independently reviewed entries.

Wildcard listeners 0.0.0.0 and [::] are allowed, as are documented example and
loopback addresses. Unbracketed all-letter forms such as a::b are treated as
ambiguous language scope names; bracket them when documenting IPv6 addresses.
Leading punctuation does not hide private hostnames. The exact .env.local
dotfile spelling is exempt from hostname detection but still rejected as a
runtime file. Only the exact lowercase .env.example template filename is
allowed; runtime env recognition is otherwise case insensitive. AI names retain
human-name handling; known assistant, agent and model qualifiers and GitHub
Copilot remain prohibited. Windsurf, Aider, Amazon Q Developer, Junie and
bracketed bot qualifiers are recognized, along with provider domains
codeium.com, windsurf.com and aider.chat.

Scanners consume complete tokens or lines without overlapping retries.
Assignment values are consumed once, IPv6 validation uses bounded candidates,
and trailer parsing splits delimiters directly. Adversarial tests scan at least
1 MB per family (4 MB for nested block scalars) with a two-second bound and a
subprocess timeout. This is a regression budget pinned by mutation tests; each
CI job has a 15-minute timeout.
This is not a throughput promise for every machine or arbitrary denylist size.

Synthetic matches in the guard tests have exact path/category/value exceptions
in scripts/publication-fixtures.mjs. These exceptions apply only to those source
files, not scratch files or Git messages. Changing them requires review; adding
an unexpected private value to the same test file still fails.

For operator-specific names/domains that generic patterns cannot know, create
.publication-denylist.local.json locally: a JSON array of nonempty strings.
Matching is case-insensitive and literal. It is gitignored and rejected if
staged, including when force-added. Missing is allowed; malformed or unreadable
fails. Denylist matches cannot be excused by fixture allowlists. The untracked
file never travels to CI; generic checks and human review remain required there.

Automatic detection cannot identify every secret, private name or semantic
network plan. Review all prose and images manually; store private evidence
outside Git. Binary assets have an independent SHA-256 allowlist and manual
publication/licence review. Video, music, render caches and real-environment
screenshots are excluded. Only .claude/settings.json is shared under .claude/;
other settings stay local.

Allowlisted binary files are not content-scanned. Leading-zero IPv4 notation and
mixed-script homograph email addresses are not reliably detected. These limits
require manual publication review even when all automated checks pass.
