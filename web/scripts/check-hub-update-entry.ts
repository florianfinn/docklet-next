// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original production source-map proof of outage-critical entry-chunk ownership.


import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// Run after `pnpm -C web run build --sourcemap`: node web/scripts/check-hub-update-entry.ts. Unit tests need no build artefacts.
const dist = resolve(import.meta.dirname, "../dist");
const html = readFileSync(resolve(dist, "index.html"), "utf8");
const entry = html.match(/<script[^>]+src="\/([^"]+\.js)"/)?.[1];
assert.equal(typeof entry, "string", "Production index must name its entry chunk");
const map = JSON.parse(readFileSync(resolve(dist, `${entry}.map`), "utf8")) as { sources: string[] };
assert.equal(map.sources.some(path => path.endsWith("/shared/components/hub-update/HubUpdateOverlay.tsx")), true, "HubUpdateOverlay must ship in the entry chunk");
assert.equal(map.sources.some(path => path.endsWith("/shared/brand/BrandLogo.tsx")), true, "Inline BrandLogo must ship in the entry chunk");
console.log("B5 production entry contains HubUpdateOverlay and inline BrandLogo.");
