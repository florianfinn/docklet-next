/** Trims, lowercases and de-duplicates labels; keeps the entered order. */
export function normalizeStackLabels(labels: readonly string[]): string[] {
  return [...new Set(labels.map((l) => l.trim().toLowerCase()).filter(Boolean))];
}

/** Compares two label lists as normalized sets. */
export function sameStackLabels(a: readonly string[], b: readonly string[]): boolean {
  const x = normalizeStackLabels(a);
  const y = new Set(normalizeStackLabels(b));
  return x.length === y.size && x.every((l) => y.has(l));
}
