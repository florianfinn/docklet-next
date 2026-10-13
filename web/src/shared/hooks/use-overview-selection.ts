import { useCallback, useState } from "react";

import type { OverviewMode } from "@/shared/preferences/preferences-store";

/**
 * Row selection of an overview. Only Advanced selects; leaving it drops the
 * selection so no hidden rows stay targeted by bulk actions.
 */
export function useOverviewSelection(mode: OverviewMode) {
  const selecting = mode === "advanced";
  const [selected, setSelected] = useState<string[]>([]);
  if (!selecting && selected.length) setSelected([]);
  const onSelect = useCallback((id: string, on: boolean) => setSelected((s) => (on ? [...new Set([...s, id])] : s.filter((x) => x !== id))), []);
  const clear = useCallback(() => setSelected([]), []);
  return { selecting, selected: selecting ? selected : [], setSelected, onSelect, clear };
}
