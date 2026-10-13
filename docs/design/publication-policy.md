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
for local checks, and commit trees/messages/identities for history/pre-push.
PR CI scans the actual incoming head history while checking the merged working
tree separately; it does not judge GitHub's synthetic merge message.
It checks filenames as well as contents. Findings contain only path and category,
never the matching text; unsafe filenames are replaced by a fixed label. Read and
configuration errors also fail without echoing untrusted content.

Checks cover credential patterns and literal secret assignments, non-example
email, non-example IPv4/IPv6 addresses, private host suffixes, personal absolute
paths, runtime env files/private artefacts, AI co-authors, session links, invalid
UTF-8, U+FFFD, unexpected control characters and double-encoded UTF-8. Zero-width
and bidi controls U+200B-200F, U+202A-202E, U+2060-2069 and U+FEFF are rejected
along with U+00AD, U+034F, U+061C, U+115F/1160, U+180E, U+2028/2029,
U+3164 and U+FFA0 in content, filenames and Git messages. Tests construct these characters from
escaped literals or code points; no raw control-character exception is configured.
Loopback
addresses may be used only for synthetic local test listeners. RFC 5737 and
2001:db8::/32 are the network examples. Human Git identities use
users.noreply.github.com; noreply@github.com is allowed for GitHub commits.
Reserved example-domain emails are allowed in content, never as commit identities.
No address exception permits an AI identity or AI co-author.

Secret assignment names are case insensitive and match any occurrence of secret,
password, passwd, passphrase, pwd, credentials, token, api_key, apikey,
access_key, accessKey or private_key, including prefixes,
suffixes, client_secret and hyphen-separated API/private-key forms. Assignments
using equals or colon, JSON/YAML keys and double-dash flags with equals or a
space-separated value are checked, with or without quotes. Indented YAML values
and block scalars are checked too. Source files (including Go and TypeScript)
check quoted and template literals while allowing identifiers, declarations, calls,
member access, type annotations and comparisons. Bare env/YAML/flag values remain
checked; unquoted boolean/null configuration values are not credential strings.
This is lexical context recognition, not a language parser; semantic review remains
necessary, particularly for constructed strings and ambiguous prose. Every nonempty literal
value is rejected, including short values. The exact placeholder forms are empty,
changeme, example, test-only (case insensitive), all-asterisk masks, angle-bracket
placeholders, environment substitutions of the form ${VAR} or ${VAR:-default} (also shell +, ? and =
operators with a colon), and GitHub expression
substitutions. Arbitrary repeated letters and ordinary words are not placeholders.

Wildcard listeners 0.0.0.0 and [::] are allowed, as are documented example and
loopback addresses. Unbracketed all-letter forms such as a::b are treated as
ambiguous language scope names; bracket them when documenting IPv6 addresses.
Leading punctuation does not hide private hostnames. The exact .env.local dotfile
spelling is exempt from hostname detection but still rejected as a runtime file.
Only the exact lowercase .env.example template filename is allowed; runtime env
recognition is otherwise case insensitive. AI names retain human-name handling;
known assistant, agent and model qualifiers and GitHub Copilot remain prohibited.
Windsurf, Aider, Amazon Q Developer, Junie and bracketed bot qualifiers are
recognized, along with provider domains codeium.com, windsurf.com and aider.chat.

Scanners consume complete tokens or lines without overlapping retries. Assignment
values are consumed once, IPv6 validation uses bounded candidates, and trailer
parsing splits delimiters directly. Adversarial tests scan at least 1 MB per
family with a two-second bound and a subprocess timeout. This is a regression
budget pinned by mutation tests; each CI job has a 15-minute timeout. This is
not a throughput promise for every machine or arbitrary denylist size.

Synthetic matches in the guard tests have exact path/category/value exceptions
in scripts/publication-fixtures.mjs. These exceptions apply only to those source
files, not scratch files or Git messages. Changing them requires review; adding
an unexpected private value to the same test file still fails.

For operator-specific names/domains that generic patterns cannot know, create
.publication-denylist.local.json locally: a JSON array of nonempty strings. Matching
is case-insensitive and literal. It is gitignored and rejected if staged, including
when force-added. Missing is allowed; malformed or unreadable fails. Denylist
matches cannot be excused by fixture allowlists. The untracked file never travels
to CI; generic checks and human review remain required there.

Automatic detection cannot identify every secret, private name or semantic network
plan. Review all prose and images manually; store private evidence outside Git.
Binary assets have an independent SHA-256 allowlist and manual publication/licence
review. Video, music, render caches and real-environment screenshots are excluded.
Only .claude/settings.json is shared under .claude/; other settings stay local.

Allowlisted binary files are not content-scanned. Leading-zero IPv4 notation and
mixed-script homograph email addresses are not reliably detected. These limits
require manual publication review even when all automated checks pass.
