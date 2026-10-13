export type DiffLine = { kind: "same" | "add" | "del"; text: string };

/** LCS line diff; inputs are small Compose files, so O(n·m) is fine. */
export function diffLines(before: string, after: string): DiffLine[] {
  const a = before.split("\n");
  const b = after.split("\n");
  const n = a.length;
  const m = b.length;
  const lcs: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    const row = lcs[i] as number[];
    const next = lcs[i + 1] as number[];
    for (let j = m - 1; j >= 0; j--) row[j] = a[i] === b[j] ? (next[j + 1] ?? 0) + 1 : Math.max(next[j] ?? 0, row[j + 1] ?? 0);
  }
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: "same", text: a[i] ?? "" });
      i++;
      j++;
    } else if ((lcs[i + 1]?.[j] ?? 0) >= (lcs[i]?.[j + 1] ?? 0)) {
      out.push({ kind: "del", text: a[i++] ?? "" });
    } else {
      out.push({ kind: "add", text: b[j++] ?? "" });
    }
  }
  while (i < n) out.push({ kind: "del", text: a[i++] ?? "" });
  while (j < m) out.push({ kind: "add", text: b[j++] ?? "" });
  return out;
}

/** Keeps changed lines with a little context; collapses long unchanged runs. */
export function condenseDiff(lines: DiffLine[], context = 2): (DiffLine | { kind: "gap"; count: number })[] {
  const keep = lines.map((l, i) => l.kind !== "same" || lines.slice(Math.max(0, i - context), i + context + 1).some((x) => x.kind !== "same"));
  const out: (DiffLine | { kind: "gap"; count: number })[] = [];
  let gap = 0;
  lines.forEach((l, i) => {
    if (keep[i]) {
      if (gap) out.push({ kind: "gap", count: gap });
      gap = 0;
      out.push(l);
    } else gap++;
  });
  if (gap) out.push({ kind: "gap", count: gap });
  return out;
}
