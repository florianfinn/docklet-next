// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original one-time import consumer with identity and draft ownership. M19 web consumer integration.

import { parseDocument } from "yaml";
import type { ImportResult } from "@/data/adapter";

export function applyImportProposal(result: ImportResult, currentName: string, env: string) {
  const doc = parseDocument(result.composeContent);
  const name = currentName.trim() || result.title;
  if (name) doc.set("name", name);
  const present = new Set(env.split(/\r?\n/).map((line) => line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=/)?.[1]));
  const additions = [...new Set(result.envNames)].filter((key) => !present.has(key)).map((key) => `${key}=`);
  return { compose: doc.toString(), env: additions.length ? `${env.trimEnd()}${env.trimEnd() ? "\n" : ""}${additions.join("\n")}\n` : env };
}
