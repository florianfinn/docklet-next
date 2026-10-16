# Arcane SQLite reference and M1 removal record

The [SQLite hub decision](sqlite-database.md) supersedes the PostgreSQL-only
removal policy. Its driver, Francis restoration and fresh baseline sections
define what implementation step 2 recovers and what stays excluded.

The exact M1 removal inventory, including all 90 upstream migration paths,
remains in the [historical removal map](../history/m1-sqlite-removal.md).
Those paths refer to Arcane v2.15.1 at
`095cb778daba1034e2a39961199e712a75d4ba29`; the
[adoption record](arcane-adoption.md) holds the module and licence references.
The old migrations and legacy database import are not restored.
