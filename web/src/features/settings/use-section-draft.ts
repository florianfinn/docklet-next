import { useCallback, useState } from "react";

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

const isRecord = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * Local draft of one settings section. A new saved value replaces the draft
 * only while it is clean, so a background refetch never wipes edits. With
 * rebase "fields", fields changed elsewhere are taken over even while dirty
 * and the other edits stay.
 */
export function useSectionDraft<T>(saved: T | undefined, options?: { rebase?: "fields" }) {
  const [base, setBase] = useState<T | undefined>(saved);
  const [draft, setDraft] = useState<T | undefined>(saved);
  const dirty = draft !== undefined && base !== undefined && !same(draft, base);

  if (saved !== undefined && !same(saved, base)) {
    if (!dirty) {
      setBase(saved);
      setDraft(saved);
    } else if (options?.rebase === "fields" && saved && base && draft) {
      const changed = (Object.keys(saved) as (keyof T)[]).filter((k) => !same(saved[k], base[k]));
      setBase(saved);
      setDraft({ ...draft, ...Object.fromEntries(changed.map((k) => [k, saved[k]])) });
    }
  }

  const patch = useCallback((p: Partial<T>) => setDraft((d) => (d === undefined ? d : { ...d, ...p })), []);
  const reset = useCallback(() => setDraft(base), [base]);
  /**
   * Marks a successful save. `saved` is the stored value, `submitted` what was
   * sent. Input typed since then stays: plain objects take `saved` for fields
   * untouched since submit, arrays keep the whole draft.
   */
  const commit = useCallback((saved: T, submitted: T = saved) => {
    setBase(saved);
    setDraft((d) => {
      if (d === undefined || same(d, submitted)) return saved;
      if (!isRecord(d) || !isRecord(saved) || !isRecord(submitted)) return d;
      const next: Record<string, unknown> = { ...saved };
      for (const k of Object.keys(d)) if (!same(d[k], submitted[k])) next[k] = d[k];
      return next as T;
    });
  }, []);

  return { draft, setDraft, patch, dirty, reset, commit };
}
