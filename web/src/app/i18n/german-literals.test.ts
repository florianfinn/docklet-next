import ts from "typescript";
import { describe, expect, it } from "vitest";

const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}", "!/src/**/*.d.ts", "!/src/**/messages/**", "!/src/test/**"], { query: "?raw", import: "default", eager: true });
const GERMAN = /[äöüÄÖÜß„]|\b(und|oder|nicht|der|die|das|den|dem|des|ein|eine|kein|keine|mit|von|für|auf|über|ist|sind|wird|werden|nach|bei|aus|zum|zur|noch|alle|ohne|unter|bitte|neu|Fehler|Neuer|Direkt|gerade|eben|gleich|jetzt|Tag|Tage|Tagen|Lädt)\b/;

/**
 * Files whose German literals are data, not interface text. Demo backend texts
 * are translated at display time (data/demo/demo-text) or are raw output, logs
 * and file contents; the formatter keeps its German words beside the English ones.
 */
const DATA = [/^src\/data\/demo\//, /^src\/features\/dashboard\/services\/service-data\.ts$/, /^src\/features\/activity\/sample-run\.ts$/, /^src\/features\/stacks\/editor\/new-stack-draft\.ts$/];
const ALLOWED = new Map([
  ["src/domain/format.ts", ["gerade eben", "gleich", "jetzt", "Tag", "Tagen"]],
  // Only reserves layout width for the localized label (networks.hostAddressUnknown).
  ["src/features/networks/topology-layout.ts", ["Host-Adresse unbekannt"]],
]);

function germanLiterals(file: string, source: string): string[] {
  const sf = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, file.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    const text = ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node) ? node.text : ts.isTemplateExpression(node) ? node.getText(sf) : ts.isJsxText(node) ? node.text.trim() : undefined;
    if (text !== undefined && !ts.isImportDeclaration(node.parent) && !ts.isExportDeclaration(node.parent)) {
      if (GERMAN.test(text)) found.push(text);
      if (ts.isTemplateExpression(node)) return;
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return found;
}

describe("interface source", () => {
  it("has no German literals outside the language files, except documented data", () => {
    const files = Object.keys(sources).map((p) => p.slice(1));
    expect(files.length).toBeGreaterThan(300);
    const hits = files
      .filter((f) => !DATA.some((re) => re.test(f)))
      .flatMap((f) => germanLiterals(f, sources[`/${f}`]!).filter((text) => !ALLOWED.get(f)?.includes(text)).map((text) => `${f}: ${text}`));
    expect(hits).toEqual([]);
  });

  it("would notice German in attributes, JSX text and templates", () => {
    expect(germanLiterals("probe.tsx", 'const a = <p title="Neuer Stack">Lädt</p>; const b = `${n} Tage`; const c = "Docker Hub · anonymous";')).toEqual(["Neuer Stack", "Lädt", "`${n} Tage`"]);
  });
});
