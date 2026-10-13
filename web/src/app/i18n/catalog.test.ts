import { createTranslator } from "use-intl";
import { describe, expect, it } from "vitest";

import { catalogs } from "./catalog";
import { messages } from "./messages";

type Tree = { [key: string]: string | string[] | Tree };

/** Argument names, types and rich-text tags of an ICU message, quoted text skipped. */
function icuShape(message: string): string[] {
  const out = new Set<string>();
  const walk = (s: string, inPlural: boolean) => {
    let i = 0;
    while (i < s.length) {
      const c = s[i]!;
      if (c === "'" && s[i + 1] === "'") i += 2;
      else if (c === "'" && /[{}#|<]/.test(s[i + 1] ?? "")) i = s.indexOf("'", i + 1) < 0 ? s.length : s.indexOf("'", i + 1) + 1;
      else if (c === "<") {
        const m = /^<\/?([a-zA-Z0-9]+)>/.exec(s.slice(i));
        if (m) out.add(`<${m[1]}>`);
        i += m ? m[0].length : 1;
      } else if (c === "{") {
        let depth = 1;
        let j = i + 1;
        while (j < s.length && depth) {
          if (s[j] === "{") depth++;
          else if (s[j] === "}") depth--;
          j++;
        }
        const body = s.slice(i + 1, j - 1);
        const [name = "", type = "", ...rest] = body.split(",");
        out.add(`${name.trim()}:${type.trim()}`);
        if (/^(plural|select|selectordinal)$/.test(type.trim())) {
          // Option bodies are sub-messages; compare their arguments, not the branch keys.
          const options = rest.join(",");
          let k = 0;
          while (k < options.length) {
            const open = options.indexOf("{", k);
            if (open < 0) break;
            let d = 1;
            let e = open + 1;
            while (e < options.length && d) {
              if (options[e] === "{") d++;
              else if (options[e] === "}") d--;
              e++;
            }
            walk(options.slice(open + 1, e - 1), type.trim() !== "select" || inPlural);
            k = e;
          }
        }
        i = j;
      } else i++;
    }
  };
  walk(message, false);
  return [...out].sort();
}

function leaves(tree: Tree | string | string[], path = ""): [string, string][] {
  if (typeof tree === "string") return [[path, tree]];
  if (Array.isArray(tree)) return tree.map((v, i) => [`${path}[${i}]`, v]);
  return Object.entries(tree).flatMap(([k, v]) => leaves(v, path ? `${path}.${k}` : k));
}

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

  it("formats every message without missing arguments or errors", () => {
    const errors: string[] = [];
    const t = createTranslator({ locale: "en-US", messages: catalogs["en-US"], timeZone: "Europe/Berlin", onError: (e) => errors.push(e.message) });
    for (const [key, text] of deLeaves) {
      if (key.includes("[")) continue;
      const values: Record<string, unknown> = {};
      for (const part of icuShape(text)) {
        if (part.startsWith("<")) values[part.slice(1, -1)] = (chunks: unknown) => chunks;
        else {
          const [name, type] = part.split(":");
          values[name!] = type === "plural" || type === "number" || type === "selectordinal" ? 2 : type === "date" || type === "time" ? new Date(0) : "x";
        }
      }
      const translate = t as unknown as { (k: string, v: unknown): unknown; rich: (k: string, v: unknown) => unknown };
      const out = icuShape(text).some((p) => p.startsWith("<")) ? translate.rich(key, values) : translate(key, values);
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

// SHA-256 of the German catalogue files before English was added; German texts stay byte-identical.
const GERMAN_FILES: Record<string, string> = {
  "src/shared/help/messages/de.ts": "6c4d56088f30bee81bef7790554c0b7d5afda2ea0f17b29669471e25c3e959cc",
  "src/shared/workspace/messages/de.ts": "36b5c87153633eb2951721264481f743c430669c6a9111d70125f5cc164240eb",
  "src/shared/theme/messages/de.ts": "c5527ec5e0ac2c15a1a4533aa6fc385e07a633b28be4cdebcbd5b14d97470379", // Approved change: Arcane description without "Docklet-Standard".
  "src/shared/messages/de.ts": "ea80e44e5382800b49dd825b3d3f9e6707add4aa100a5d39624a986d71e46d45",
  "src/features/setup/messages/de.ts": "103773719a6c6d5741632d0ad60a9029a04594efb578f64719f400db4367c565",
  "src/features/auth/login/messages/de.ts": "fc18b4ff9a7997507dc122d1f7fdfa00766e250cb99a2015d45f27f432129250",
  "src/features/images/messages/de.ts": "1d889215efdf9c8103b2108d8bea49dc366922e1b41c4dc6f2998b1d02380030",
  "src/features/activity/messages/de.ts": "c9f5297bcba9171fbb364e5f58b606c48e1d7d10e890ce9b2747bae6b0f21df8",
  "src/features/containers/messages/de.ts": "47fa74a7f265ca7fbe7f7d42ed9513a4dcc2d2e6c6da1ea0a935dbcd4c5c571d",
  "src/features/networks/messages/de.ts": "c29ba24c6834b5406ead815a2e260103b9c91eeaa447cd78c196f4867ab64dce",
  "src/features/hosts/messages/de.ts": "ddd3810607eb5094002af5ba297ec0bc823d4f8dc397e416363215e4599918f3",
  "src/features/security/messages/de.ts": "74c9115bc1eefadb4c0de0c2322d709bdbde87d0dec0cb4318363bdfcbb86dd1",
  "src/features/dashboard/messages/de.ts": "7689127e3c7df69cc342990e1f0b8685434c934edecaac243c3250e9a6215f64",
  "src/features/design-lab/messages/de.ts": "f941181f5098d40bed583469bc7c2729dc6c0891cf46d59a4a6022865b5f206c",
  "src/features/design-lab/messages/visual-de.ts": "8af269911687ecf5fc9ec4be6dd7ed05ad6fd210a25e018dc7c23a16313d0443",
  "src/features/settings/messages/de.ts": "cb93d80db5b6979f6ae3f3253884563643ea35ba57ac2588a380d7ed63713312",
  "src/features/stacks/editor/messages/de.ts": "697291f92d37180a45c0189ef149032bc03218f738f3864072208ac4773a9e28",
  "src/features/stacks/messages/de.ts": "3b038aae5bc4f534ad39cc5a9a7138ea098bb2eee99dd310652fb1ccdab7d6b7",
  "src/features/volumes/messages/de.ts": "d95f6f1f430165044f853a757b05ee3746dc60e9ee7b05e598b7fbb5abef2163",
  "src/app/shell/messages/de.ts": "44a851951c82b6e8436e9757815305a4fb8ee276ca95831902245b096fe7e362",
  "src/app/preview/messages/de.ts": "e9115528589dc3e1904da30c808b3dbbb164fbb79daaf28509812c77a145d2e5",
};

const germanSources = import.meta.glob<string>("/src/**/messages/*de.ts", { query: "?raw", import: "default", eager: true });

async function sha256(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

describe("German catalogue", () => {
  it("keeps every pre-English German file byte-identical", async () => {
    const changed: string[] = [];
    for (const [path, sha] of Object.entries(GERMAN_FILES)) if ((await sha256(germanSources[`/${path}`] ?? "")) !== sha) changed.push(path);
    expect(changed).toEqual([]);
    // German files added since: the language switch namespace and the address-pool errors.
    expect(Object.keys(germanSources).filter((p) => !(p.slice(1) in GERMAN_FILES)).sort()).toEqual(["/src/app/i18n/messages/de.ts", "/src/features/networks/messages/pool-de.ts"]);
  });

  it("is still the German catalogue the app renders, extended only by the language switch namespace", () => {
    expect(catalogs.de).toBe(messages);
    const { locale, ...older } = messages;
    expect(Object.keys(older)).toHaveLength(21);
    expect(leaves(locale as unknown as Tree).map(([, v]) => v)).toContain("Sprache");
  });
});
