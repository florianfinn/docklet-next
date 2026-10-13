#!/usr/bin/env python3
"""Turn a schema-only pg_dump of the model registry into one Goose migration."""

import re
import sys

source = sys.stdin.read()
source = "\n".join(line for line in source.splitlines() if not line.startswith("--"))
statements = []
for chunk in source.split(";"):
    statement = chunk.strip()
    if re.match(r"^(CREATE (?:TABLE|SEQUENCE|(?:UNIQUE )?INDEX)|ALTER (?:TABLE|SEQUENCE))\b", statement):
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
