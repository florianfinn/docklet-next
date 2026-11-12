// @vitest-environment node
// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original lazy terminal payload budget over the production build graph.

import { gzipSync } from "node:zlib";
import { build } from "vite";
import { expect, it, vi } from "vitest";

it("keeps xterm out of startup and the terminal payload below 160 KiB gzip", async () => {
  const root = process.cwd();
  // Vite derives DEV from NODE_ENV; a production graph must use production,
  // even when this check is hosted by a Vitest worker.
  vi.stubEnv("NODE_ENV", "production");
  let result;
  try { result = await build({ root, configFile: `${root}/vite.config.ts`, mode: "production", logLevel: "silent", build: { write: false } }); } finally { vi.unstubAllEnvs(); }
  if (Array.isArray(result) || !("output" in result)) throw new Error("missing build output");
  const chunks = result.output.filter((entry) => entry.type === "chunk");
  const terminal = chunks.filter((chunk) => Object.keys(chunk.modules).some((id) => id.includes("/@xterm/") || id.endsWith("/ContainerTerminal.tsx")));
  expect(terminal.length).toBeGreaterThan(0);
  const startup = chunks.filter((chunk) => chunk.isEntry || Object.keys(chunk.modules).some((id) => id.endsWith("/ContainerTerminalTabs.tsx")));
  const byName = new Map(chunks.map((chunk) => [chunk.fileName, chunk]));
  const staticallyLoaded = new Set<string>();
  const visit = (name: string) => { if (staticallyLoaded.has(name)) return; staticallyLoaded.add(name); byName.get(name)?.imports.forEach(visit); };
  startup.forEach((chunk) => visit(chunk.fileName));
  for (const chunk of terminal) expect(staticallyLoaded.has(chunk.fileName)).toBe(false);
  const styles = result.output.filter((entry) => entry.type === "asset").filter((entry) => entry.fileName.endsWith(".css") && String(entry.source).includes(".xterm"));
  expect(styles.length).toBeGreaterThan(0);
  const bytes = terminal.reduce((sum, chunk) => sum + gzipSync(chunk.code).byteLength, 0) + styles.reduce((sum, asset) => sum + gzipSync(asset.source).byteLength, 0);
  expect(bytes).toBeLessThanOrEqual(160 * 1024);
  console.log(JSON.stringify({ terminalGzipBytes: bytes, terminalChunks: terminal.map((chunk) => chunk.fileName) }));
});
