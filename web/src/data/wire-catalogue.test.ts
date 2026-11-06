// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { GUARDED_WIRE_ENUMS, EXEMPT_WIRE_ENUMS } from "./wire-catalogue.test-support";

type Node = { properties?: Record<string, Node>; items?: Node; allOf?: Node[]; anyOf?: Node[]; oneOf?: Node[]; enum?: unknown[]; $ref?: string };
const PROPERTY_NAMES = new Set(["code", "reason", "errorCode", "messageCode", "baselineCode", "pointCode", "policyCode", "status", "state", "kind", "outcome", "phase"]);

function enumProperties(schemas: Record<string, Node>): Map<string, string[]> {
  const result = new Map<string, string[]>();
  function walk(node: Node, path: string) {
    for (const [name, child] of Object.entries(node.properties ?? {})) {
      const property = `${path}.${name}`;
      if (PROPERTY_NAMES.has(name) && child.enum?.length && child.enum.every((v) => typeof v === "string")) {
        expect(result.has(property), `duplicate schema property ${property}`).toBe(false);
        result.set(property, child.enum as string[]);
      }
      walk(child, property);
    }
    if (node.items) walk(node.items, `${path}[]`);
    for (const composition of ["allOf", "anyOf", "oneOf"] as const) {
      node[composition]?.forEach((child, index) => walk(child, `${path}.${composition}[${index}]`));
    }
    // Reference targets are visited once under their own component schema identity.
  }
  for (const [name, schema] of Object.entries(schemas)) walk(schema, name);
  return result;
}

const contract = JSON.parse(readFileSync(resolve(process.cwd(), "../api/openapi.json"), "utf8"));
const properties = enumProperties(contract.components.schemas);

describe("rendered wire enum catalogue classification", () => {
  it("classifies every contract enum property exactly once, without stale entries", () => {
    const classified: string[] = [...GUARDED_WIRE_ENUMS.flatMap((family) => [...family.properties]), ...EXEMPT_WIRE_ENUMS.map((entry) => entry.property)];
    expect(classified.length).toBe(new Set(classified).size);
    expect(classified.toSorted()).toEqual([...properties.keys()].toSorted());
    for (const entry of EXEMPT_WIRE_ENUMS) expect(entry.reason.trim().length).toBeGreaterThan(20);
  });

  it.each(GUARDED_WIRE_ENUMS)("pins all members of $name to its actual total map", (family) => {
    const members = new Set(family.properties.flatMap((property) => properties.get(property) ?? []));
    expect(Object.keys(family.keys).toSorted()).toEqual([...members].toSorted());
  });

  it("walks nested objects, array items and all composition forms without expanding references", () => {
    const field = { properties: { code: { enum: ["example"] } } };
    expect([...enumProperties({
      Nested: { properties: { envelope: { properties: { items: { items: field } } }, status: { enum: [1, 2] }, ignored: { enum: ["example"] } }, allOf: [field], anyOf: [field], oneOf: [field] },
      Reference: { $ref: "#/components/schemas/Target" }, Target: field,
    }).keys()]).toEqual(["Nested.envelope.items[].code", "Nested.allOf[0].code", "Nested.anyOf[0].code", "Nested.oneOf[0].code", "Target.code"]);
  });
});
