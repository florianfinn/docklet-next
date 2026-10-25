import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { enBackend } from "./en";
import { deBackend } from "./de";
const fixture = JSON.parse(readFileSync(resolve(process.cwd(), "../types/contract/testdata/agent-hub-v7.json"), "utf8")) as { imageSafety: { errorCodes: string[] } };
const get = (tree: unknown, key: string): unknown => key.split(".").reduce<unknown>((value, segment) => value && typeof value === "object" ? (value as Record<string, unknown>)[segment] : undefined, tree);
describe("v7 image and registry code catalogue", () => {
  it.each(fixture.imageSafety.errorCodes)("translates %s in both locales", (code) => {
    expect(get(enBackend.errors, code)).toBeTypeOf("string");
    expect(get(deBackend.errors, code)).toBeTypeOf("string");
    expect(get(enBackend.errors, code)).not.toBe("");
    expect(get(deBackend.errors, code)).not.toBe("");
  });
});

describe("actionable image and registry outcomes", () => {
  it.each([enBackend, deBackend])("keeps different recovery actions distinct", (catalogue) => {
    const messages = fixture.imageSafety.errorCodes.map((code) => get(catalogue.errors, code));
    expect(new Set(messages).size).toBe(messages.length);
    expect(catalogue.errors.registry["authentication-required"]).not.toBe(catalogue.errors.registry["authentication-failed"]);
    expect(catalogue.errors.update["deploy-failed"]).not.toBe(catalogue.errors.image["pull-failed"]);
  });
});
