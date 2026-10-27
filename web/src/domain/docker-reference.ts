// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original Docker reference identity normalization for issue #231.

const COMPONENT = /^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$/;
const TAG = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/;
const DOMAIN = /^(?:[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?|\[[a-fA-F0-9:]+\])(?::[0-9]+)?$/;
const DIGEST = /^[a-zA-Z][a-zA-Z0-9]*(?:[+._-][a-zA-Z][a-zA-Z0-9]*)*:[a-fA-F0-9]{32,}$/;

/** Docker identity retains explicit tags/digests; only registry/namespace/tag defaults expand. */
export function normalizeDockerReference(input: string): string | undefined {
  const value = input.trim(), pieces = value.split("@");
  if (!value || pieces.length > 2 || (pieces.length === 2 && !DIGEST.test(pieces[1]!))) return undefined;
  let name = pieces[0]!, tag: string | undefined;
  const colon = name.lastIndexOf(":");
  if (colon > name.lastIndexOf("/")) { tag = name.slice(colon + 1); name = name.slice(0, colon); if (!TAG.test(tag)) return undefined; }
  const parts = name.split("/");
  const first = parts[0]!;
  const hasRegistry = parts.length > 1 && (/[.:]/.test(first) || first === "localhost" || first !== first.toLowerCase());
  let registry = hasRegistry ? parts.shift()!.toLowerCase() : "docker.io";
  if (registry === "index.docker.io") registry = "docker.io";
  if (!DOMAIN.test(registry) || parts.some((part) => !COMPONENT.test(part))) return undefined;
  if (registry === "docker.io" && parts.length === 1) parts.unshift("library");
  if (!tag && pieces.length === 1) tag = "latest";
  return `${registry}/${parts.join("/")}${tag ? `:${tag}` : ""}${pieces.length === 2 ? `@${pieces[1]}` : ""}`;
}

export function sameImageReference(a: string, b: string): boolean {
  const left = normalizeDockerReference(a), right = normalizeDockerReference(b);
  return left !== undefined && right !== undefined && left === right;
}

/** Keep an exact producer key when present, then match equivalent Docker spellings. */
export function imageReferenceValue<T>(values: Record<string, T> | undefined, reference: string): T | undefined {
  if (!values) return undefined;
  if (Object.hasOwn(values, reference)) return values[reference];
  return Object.entries(values).find(([key]) => sameImageReference(key, reference))?.[1];
}
