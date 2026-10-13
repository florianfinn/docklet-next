/// <reference types="node" />
import { createHash } from "node:crypto";

import { describe, expect, it } from "vitest";

import { uiLiteralExceptions } from "./ui-literal-exceptions.test-support";
import { uiLiterals } from "./ui-literals.test-support";

const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}", "!/src/**/*.test-support.ts", "!/src/**/*.d.ts", "!/src/**/messages/**", "!/src/test/**"], { query: "?raw", import: "default", eager: true });

// Demo backend messages are localized at display time or represent raw logs and files.
const DATA = [/^src\/data\/demo\//, /^src\/features\/dashboard\/services\/service-data\.ts$/, /^src\/features\/activity\/sample-run\.ts$/, /^src\/features\/stacks\/editor\/new-stack-draft\.ts$/];
const sha256 = (text: string) => createHash("sha256").update(text).digest("hex");

function checkSources(input: Record<string, string>, exceptions = uiLiteralExceptions): string[] {
  const counts = new Map<string, number>();
  for (const [path, source] of Object.entries(input)) {
    const file = path.replace(/^\//, "");
    if (DATA.some((pattern) => pattern.test(file))) continue;
    const lines = source.split(/\r\n|\r|\n/);
    for (const hit of uiLiterals(file, source)) {
      const key = `${file}:${sha256(lines[hit.line - 1]!)}`;
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  const failures: string[] = [];
  const used = new Set<string>();
  for (const entry of exceptions) {
    const key = `${entry.file}:${entry.lineSha256}`;
    if (used.has(key) || entry.reason.trim().length < 12 || counts.get(key) !== entry.count) failures.push(`${entry.file}: stale or invalid exception`);
    used.add(key);
    counts.delete(key);
  }
  for (const key of counts.keys()) failures.push(`${key.split(":")[0]}: UI literal outside catalogues`);
  return failures;
}

describe("interface source", () => {
  it("has no UI literals outside catalogues beyond exact reviewed import exceptions", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(300);
    expect(checkSources(sources)).toEqual([]);
  });

  it.each([
    ["English JSX", '<p>Loading</p>'],
    ["German JSX", '<p>Lädt</p>'],
    ["French JSX", '<p>Chargement</p>'],
    ["Japanese JSX", '<p>読み込み</p>'],
    ["aria label", '<button aria-label="Reload" />'],
    ["title", '<button title="Reload" />'],
    ["expression title", '<button title={"Reload"} />'],
    ["conditional JSX", '<p>{ready ? "Ready" : "Waiting"}</p>'],
    ["template JSX", '<p>{`Loading ${count}`}</p>'],
    ["nested template JSX", '<p>{`${ready ? "Ready" : "Waiting"}`}</p>'],
    ["text option", 'const option = { label: "Reload" };'],
    ["indirect prose", 'const text = "Reload the page";'],
  ])("rejects an injected %s literal (red proof)", (_name, source) => {
    expect(checkSources({ "/src/probe.tsx": source }, [])).toEqual(["src/probe.tsx: UI literal outside catalogues"]);
  });

  it("rejects injected JSX and attributes in a real scanned file", () => {
    for (const injected of ['const probe = <p>Loading</p>;', 'const probe = <p title="Reload" aria-label="Refresh" />;']) {
      const mutated = { ...sources, "/src/app/i18n/LanguageSwitch.tsx": `${sources["/src/app/i18n/LanguageSwitch.tsx"]}\n${injected}` };
      expect(checkSources(mutated)).toEqual(["src/app/i18n/LanguageSwitch.tsx: UI literal outside catalogues"]);
    }
  });

  it("does not mistake translation keys, classes, comparisons or dynamic values for UI literals", () => {
    expect(uiLiterals("probe.tsx", 'const a = <p className="flex items-center" title={t("title")}>{kind === "ready" && t("ready")}{`${t("label")}: ${name}`}</p>;')).toEqual([]);
  });

  it("rejects stale exceptions and additional literals on an excepted line", () => {
    const source = '<p>Close</p>;', file = "src/probe.tsx";
    const exceptions = [{ file, lineSha256: sha256(source), count: 1, reason: "Existing imported component text." }];
    expect(checkSources({ [file]: source }, exceptions)).toEqual([]);
    expect(checkSources({ [file]: "" }, exceptions).length).toBe(1);
    expect(checkSources({ [file]: `${source}<p>Open</p>;` }, exceptions).length).toBe(2);
  });
});
