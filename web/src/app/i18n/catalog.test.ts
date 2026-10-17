/// <reference types="node" />
import { createHash } from "node:crypto";

import { createTranslator } from "use-intl";
import { describe, expect, it } from "vitest";

import { apostropheExceptions } from "./apostrophe-exceptions.test-support";
import { catalogs } from "./catalog";
import { messages } from "./messages";
import { icuShape, leaves, sampleArguments, validateCatalog, type MessageTree as Tree } from "./catalog-validation.test-support";

const de = messages as unknown as Tree;
const en = catalogs["en-US"] as unknown as Tree;
const deLeaves = new Map(leaves(de));
const enLeaves = new Map(leaves(en));

describe("English catalogue", () => {
  it("has every German namespace, key and array entry, and nothing else", () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(de).sort());
    expect([...enLeaves.keys()].sort()).toEqual([...deLeaves.keys()].sort());
    expect(deLeaves.size).toBeGreaterThan(3000);
  });

  it("uses the same ICU arguments, plural/select arguments and tags as German", () => {
    const mismatches = [...deLeaves].filter(([key, text]) => icuShape(text).join("|") !== icuShape(enLeaves.get(key) ?? "").join("|")).map(([key]) => key);
    expect(mismatches).toEqual([]);
  });

  it.each(["en-US", "de"] as const)("formats every %s message without missing arguments or errors", (locale) => {
    const errors: string[] = [];
    const t = createTranslator({ locale, messages: catalogs[locale], timeZone: "Europe/Berlin", onError: (e) => errors.push(e.message) });
    for (const [key, text] of leaves(catalogs[locale])) {
      if (key.includes("[")) continue;
      const values = sampleArguments(text);
      const translate = t as unknown as { (k: string, v: unknown): unknown; rich: (k: string, v: unknown) => unknown };
      const out = Object.values(values).some((value) => typeof value === "function") ? translate.rich(key, values) : translate(key, values);
      expect(out, key).not.toBe(key);
    }
    expect(errors).toEqual([]);
  });

  it("labels filter summary counts without depending on a plural noun", () => {
    const t = createTranslator({ locale: "en-US", messages: catalogs["en-US"], namespace: "common.columnFilters" });
    const noun = catalogs["en-US"].security.filters.findings;
    expect([0, 1, 13].map((total) => t("count", { total, noun }))).toEqual(["Findings: 0", "Findings: 1", "Findings: 13"]);
    expect([1, 2].map((filters) => t("countFiltered", { shown: 1, total: 13, noun, filters }))).toEqual(["Findings: 1 of 13 · 1 filter active", "Findings: 1 of 13 · 2 filters active"]);
  });

  it("is non-empty wherever German is and contains no German spelling", () => {
    const empty = [...deLeaves].filter(([key, text]) => text.trim() && !enLeaves.get(key)?.trim()).map(([key]) => key);
    expect(empty).toEqual([]);
    // Proper names that intentionally stay as they are.
    const keep = new Set(["shell.user.name"]);
    const german = [...enLeaves].filter(([key, text]) => !keep.has(key) && /[äöüÄÖÜß„]/.test(text)).map(([key]) => key);
    expect(german).toEqual([]);
  });
});

describe("German catalogue", () => {
  it("renders the German catalogue with the language switch and Hub backup namespace", () => {
    expect(catalogs.de).toBe(messages);
    const { locale, ...older } = messages;
    expect(Object.keys(older)).toHaveLength(24);
    expect(older.user).toEqual({ login: "Benutzer angemeldet.", logout: "Benutzer abgemeldet." });
    expect(catalogs["en-US"].user).toEqual({ login: "User signed in.", logout: "User signed out." });
    expect(older.hubBackups.title).toBe("Backups der Hub-Datenbank");
    expect(leaves(locale as unknown as Tree).map(([, v]) => v)).toContain("Sprache");
  });
});

const catalogueModules = import.meta.glob<Record<string, Tree>>("/src/**/messages/*{en,de}.ts", { eager: true });

describe("catalogue contracts", () => {
  it("validates every catalogue pair, including pool and disconnected backend messages", () => {
    const englishFiles = Object.keys(catalogueModules).filter((path) => path.endsWith("en.ts"));
    expect(englishFiles).toHaveLength(26);
    expect(Object.keys(catalogueModules)).toHaveLength(52);
    const used = new Set<string>();
    const errors = englishFiles.flatMap((path) => {
      const english = Object.values(catalogueModules[path]!)[0]!;
      const german = Object.values(catalogueModules[path.replace(/en\.ts$/, "de.ts")]!)[0]!;
      return validateCatalog(english, german).filter((error) => {
        const prefix = "base.", suffix = ": apostrophe";
        if (!error.startsWith(prefix) || !error.endsWith(suffix)) return true;
        const key = error.slice(prefix.length, -suffix.length);
        const message = new Map(leaves(english)).get(key)!;
        const sha256 = createHash("sha256").update(message).digest("hex");
        const exception = apostropheExceptions.find((entry) => entry.file === path && entry.key === key && entry.sha256 === sha256);
        if (!exception) return true;
        used.add(`${path}:${key}`);
        return false;
      }).map((error) => `${path}: ${error}`);
    });
    expect(errors).toEqual([]);
    expect(used.size).toBe(apostropheExceptions.length);
    // Per-file validation above owns the exact imported apostrophe exceptions.
    expect(validateCatalog(en, de).filter((error) => !error.endsWith(": apostrophe"))).toEqual([]);
  });

  it.each([
    ["missing key", { hello: "Hello" }, {}, "missing key"],
    ["extra key", {}, { hello: "Hallo" }, "extra key"],
    ["empty namespace", {}, { empty: {} }, "extra key"],
    ["node type", { list: [] }, { list: {} }, "type mismatch"],
    ["missing array entry", { list: ["One", "Two"] }, { list: ["Eins"] }, "missing key"],
    ["argument name", { hello: "Hello {name}" }, { hello: "Hallo {user}" }, "mismatch"],
    ["argument type", { hello: "{count, number}" }, { hello: "{count}" }, "mismatch"],
    ["plural branch", { hello: "{n, plural, one {one} other {many}}" }, { hello: "{n, plural, other {viele}}" }, "mismatch"],
    ["select branch", { hello: "{s, select, yes {yes} other {no}}" }, { hello: "{s, select, no {nein} other {ja}}" }, "mismatch"],
    ["branch argument", { hello: "{n, plural, one {{a}} other {{b}}}" }, { hello: "{n, plural, one {{b}} other {{a}}}" }, "mismatch"],
    ["ordinal", { hello: "{n, plural, other {#}}" }, { hello: "{n, selectordinal, other {#}}" }, "mismatch"],
    ["offset", { hello: "{n, plural, offset:1 other {#}}" }, { hello: "{n, plural, other {#}}" }, "mismatch"],
    ["rich tag", { hello: "<b>Hello</b>" }, { hello: "<i>Hallo</i>" }, "mismatch"],
    ["broken ICU", { hello: "Hello" }, { hello: "{name" }, "invalid ICU"],
    ["broken English ICU", { hello: "{name" }, { hello: "Hallo" }, "invalid ICU"],
    ["broken unused branch", { hello: "{s, select, other {ok}}" }, { hello: "{s, select, yes {{x, invalid}} other {ok}}" }, "invalid ICU"],
    ["apostrophe", { hello: "Don''t" }, { hello: "Don't" }, "apostrophe"],
  ] satisfies [string, Tree, Tree, string][])("rejects %s (red proof)", (_name, base, translation, error) => {
    expect(validateCatalog(base, translation).some((entry) => entry.includes(error))).toBe(true);
  });

  it("permits doubled apostrophes, explicit ICU quoting and reordered arguments", () => {
    expect(validateCatalog({ text: "Don''t '{literal}' {a} {b}" }, { text: "{b} '{literal}' {a} Don''t" })).toEqual([]);
  });
});
