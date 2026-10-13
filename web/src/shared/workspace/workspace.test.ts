import { describe, expect, it } from "vitest";

import { languageFor } from "./languages";

const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}"], { query: "?raw", import: "default", eager: true });

describe("shared workspace module", () => {
  it("infers editor languages from file names with a plain-text fallback", () => {
    const cases: [string, string][] = [
      ["compose.yaml", "yaml"],
      ["automations.yml", "yaml"],
      ["package.json", "json"],
      [".env", "env"],
      ["prod.env", "env"],
      ["postgresql.conf", "ini"],
      ["config.toml", "ini"],
      ["install.sh", "shell"],
      ["system.xml", "text"],
      ["README", "text"],
    ];
    expect(cases.filter(([name, lang]) => languageFor(name) !== lang)).toEqual([]);
  });

  it("stays generic: no feature, Compose or volume logic inside", () => {
    const own = Object.entries(sources).filter(([path]) => path.startsWith("/src/shared/workspace/"));
    expect(own.length >= 4).toBe(true);
    const leaks = own.filter(([, code]) => /from\s+["'](@\/features|@\/domain\/(compose|volumes)|@\/data)/.test(code)).map(([p]) => p);
    expect(leaks).toEqual([]);
  });

  it("is the only CodeMirror host and is composed by both Compose and volume files", () => {
    const cmHosts = Object.entries(sources)
      .filter(([, code]) => /from\s+["']@codemirror\/view["']/.test(code))
      .map(([p]) => p);
    expect(cmHosts).toEqual(["/src/shared/workspace/code-editor.tsx"]);
    const uses = (path: string, mod: string) => !!sources[path]?.includes(`from "@/shared/workspace/${mod}"`);
    const compose = ["/src/features/stacks/editor/StackWorkspace.tsx", "/src/features/stacks/editor/AdvancedEditor.tsx", "/src/features/stacks/editor/ComposeFileTree.tsx"];
    const files = ["/src/features/volumes/files/FileWorkspace.tsx", "/src/features/volumes/files/FileBody.tsx"];
    for (const mod of ["workspace", "file-tree", "code-editor"]) {
      expect(compose.some((p) => uses(p, mod)), `compose → ${mod}`).toBe(true);
      expect(files.some((p) => uses(p, mod)), `files → ${mod}`).toBe(true);
    }
    // No standalone textarea editor for files.
    expect(Object.entries(sources).filter(([p, code]) => p.startsWith("/src/features/volumes/files/") && code.includes("Textarea")).map(([p]) => p)).toEqual([]);
  });
});
