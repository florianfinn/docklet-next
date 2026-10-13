import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

export const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
export const allowedTerms = new Set([
  "Apache-2.0", "MIT", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MPL-2.0",
  "EPL-2.0", "PostgreSQL", "LicenseRef-SQLite-Public-Domain",
]);
export function assertLicense(expression, alternative = null) {
  const terms = expression.replace(/[()]/g, "").split(/ AND | OR /);
  if (!terms.length || terms.some((term) => !allowedTerms.has(term))) throw new Error("unapproved licence expression");
  if (terms.includes("EPL-2.0") && (expression !== "EPL-2.0 OR BSD-3-Clause" || alternative !== "BSD-3-Clause"))
    throw new Error("EPL requires the reviewed BSD election");
}
export function blockedLicenses(text) {
  const plain = text.replace(/^\s*(?:\/\/|\*)\s?/gm, "").replace(/\s+/g, " ");
  return [
    ["LGPL", /\bLGPL(?:-|\b)|(?:GNU )?(?:Lesser|Library) General Public License/i],
    ["AGPL", /\bAGPL(?:-|\b)|(?:GNU )?Affero General Public License/i],
    ["GPL", /\bGPL(?:-|\b)|GNU General Public License/i],
    ["SSPL", /\bSSPL(?:-|\b)|Server Side Public License/i],
    ["BUSL", /\bBUSL(?:-|\b)|Business Source License/i],
    ["Commons-Clause", /Commons Clause/i],
    ["Elastic", /Elastic License|\bELv2\b/i],
  ].filter(([, pattern]) => pattern.test(plain)).map(([name]) => name);
}
export function classify(text) {
  const found = new Set();
  const add = (name, pattern) => { if (pattern.test(text)) found.add(name); };
  if (/Eclipse Public License/.test(text) && /Eclipse Distribution License/.test(text)) return "EPL-2.0 OR BSD-3-Clause";
  add("LicenseRef-SQLite-Public-Domain", /SQLite Is Public Domain/);
  add("BSD-3-Clause", /Eclipse Distribution License|EDL-1\.0/);
  add("MPL-2.0", /Mozilla Public\s+License[\s\S]{0,40}2\.0|SPDX-License-Identifier:.*MPL-2\.0/);
  add("Apache-2.0", /Apache License[\s,]+Version 2\.0|SPDX-License-Identifier: Apache-2\.0|covered by two different licenses: MIT and Apache/i);
  add("MIT", /Permission is hereby granted, free of charge|covered by two different licenses: MIT and Apache|licensed under the MIT License|## License\s+MIT/i);
  add("ISC", /permission to use, copy, modify, and(?:\/or)? distribute this software for any\s+purpose with or without fee/i);
  add("PostgreSQL", /Permission to use, copy, modify, and distribute this software and its documentation for any purpose, without fee, and\s+without a written agreement/i);
  // Inspect each BSD grant separately, so one file can contain both variants.
  for (const match of text.matchAll(/Redistribution and use in source and binary forms[\s\S]*?(?=THIS SOFTWARE|$)/gi))
    found.add(/Neither the name|names of its contributors|name of the author may|name of Google Inc/i.test(match[0]) ? "BSD-3-Clause" : "BSD-2-Clause");
  add("BSD-3-Clause", /SPDX-License-Identifier: BSD-3-Clause/);
  add("CC-BY-SA-4.0", /Attribution-ShareAlike 4\.0 International/);
  add("CC-BY-ND-4.0", /Attribution-NoDerivatives 4\.0 International/);
  return [...found].sort().join(" AND ") || null;
}
export const isLegalFile = (path) => !/\.(?:go|c|h|s|js|ts|yml|yaml)$/i.test(path) &&
  (/^(licen[cs]e|copying|copyright|notice)([._-]|$)/i.test(path.split("/").at(-1)) || /(?:^|\/)LICENSES\//i.test(path));
export function legalFiles(dir, recursive = false, prefix = "") {
  return readdirSync(dir, { withFileTypes: true }).flatMap((item) => {
    const path = prefix + item.name;
    if (item.isDirectory() && recursive && ![".git", "testdata", "vendor"].includes(item.name))
      return legalFiles(join(dir, item.name), true, path + "/");
    return item.isFile() && isLegalFile(path) ? [join(dir, item.name)] : [];
  }).sort();
}
export const reviews = JSON.parse(readFileSync(new URL("./arcane-license-reviews.json", import.meta.url)));
export function reviewFor(module, version, path, bytes) {
  const entry = reviews.find((r) => r.module === module && r.version === version && r.path === path);
  if (entry && (entry.sha256 !== hash(bytes) || !entry.reason)) throw new Error(`review evidence changed: ${module}/${path}`);
  return entry;
}
export function inspectLegal(module, version, path, bytes) {
  const review = reviewFor(module, version, path, bytes);
  const text = bytes.toString();
  const blocked = blockedLicenses(text);
  if (blocked.some((name) => !review?.allowedMarkers?.includes(name))) throw new Error(`blocked licence: ${module}/${path}`);
  const detectedLicense = classify(text) ?? review?.license ?? null;
  if (!detectedLicense && !review) throw new Error(`unclassified legal file: ${module}/${path}`);
  if (detectedLicense && review?.scope !== "non-code") assertLicense(detectedLicense, module === "github.com/eclipse/paho.golang" ? "BSD-3-Clause" : null);
  return { detectedLicense, ...(review ? { review: { classification: review.classification, reason: review.reason, scope: review.scope ?? "code" } } : {}) };
}

export function inspectCompiled(module, version, path, content) {
  const comments = content.match(/\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm)?.join("\n") ?? "";
  const blocked = blockedLicenses(comments);
  if (!blocked.length) return null;
  const review = reviewFor(module, version, path, content);
  if (!review || review.scope !== "compiled-header" || blocked.some((name) => !review.allowedMarkers?.includes(name)))
    throw new Error(`blocked compiled notice: ${module}/${path}`);
  return { review, comments };
}
