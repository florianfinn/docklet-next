/** Go strings.TrimSpace and Unicode simple lowercase used by NormalizeProjectTag. */
function normalizeLabel(label: string): string {
  const trimmed = label.replace(/^[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g, "");
  // Per-codepoint mapping avoids contextual final sigma; U+0130 is the only expansion.
  return Array.from(trimmed, (char) => char === "İ" ? "i" : char.toLowerCase()).join("");
}

/** Normalizes and de-duplicates labels while keeping entered order. */
export function normalizeStackLabels(labels: readonly string[]): string[] {
  return [...new Set(labels.map(normalizeLabel).filter(Boolean))];
}

/** Compares two label lists as normalized sets. */
export function sameStackLabels(a: readonly string[], b: readonly string[]): boolean {
  const x = normalizeStackLabels(a);
  const y = new Set(normalizeStackLabels(b));
  return x.length === y.size && x.every((l) => y.has(l));
}
