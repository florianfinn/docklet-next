#!/usr/bin/env python3
"""Turn a schema-only pg_dump of the model registry into one Goose migration."""

import sys

def split_sql(source):
    statements, current = [], []
    quote = None
    i = 0
    while i < len(source):
        char = source[i]
        if quote is None and source.startswith("--", i):
            end = source.find("\n", i)
            i = len(source) if end == -1 else end
            continue
        if quote is None and source.startswith("/*", i):
            end = source.find("*/", i + 2)
            if end == -1:
                raise SystemExit("unterminated SQL comment")
            i = end + 2
            continue
        if quote is None and char in ("'", '"'):
            quote = char
        elif quote == char:
            if source[i:i + 2] == char * 2:
                current.extend((char, char))
                i += 2
                continue
            quote = None
        elif quote is None and char == ";":
            statements.append("".join(current).strip())
            current = []
            i += 1
            continue
        current.append(char)
        i += 1
    if quote or "".join(current).strip():
        raise SystemExit("unterminated SQL statement")
    return [statement for statement in statements if statement]

source = "\n".join(line for line in sys.stdin.read().splitlines()
                   if not line.startswith("\\restrict") and not line.startswith("\\unrestrict"))
statements = []
for statement in split_sql(source):
    normalized = " ".join(statement.upper().split())
    if normalized.startswith("SET ") or normalized.startswith("SELECT PG_CATALOG.SET_CONFIG("):
        continue
    if not normalized.startswith(("CREATE TABLE ", "CREATE SEQUENCE ", "CREATE INDEX ",
                                  "CREATE UNIQUE INDEX ", "ALTER TABLE ", "ALTER SEQUENCE ",
                                  "CREATE TYPE ", "COMMENT ON ")):
        raise SystemExit("unsupported pg_dump statement: " + statement.split()[0])
    statements.append(statement + ";")

if not statements or not any(s.startswith("CREATE TABLE") for s in statements):
    raise SystemExit("input does not contain a PostgreSQL schema")

print("-- Source: docklet-next/backend/internal/schema/models.go")
print("-- Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71")
print("-- Retrieved: 2026-10-13")
print("-- License: Apache-2.0")
print("-- Deviations: Generated PostgreSQL baseline from the retained GORM model registry.")
print("-- Generated from backend/internal/schema.Models with PostgreSQL 18 pg_dump.")
print("-- +goose Up")
for statement in statements:
    print(statement)
