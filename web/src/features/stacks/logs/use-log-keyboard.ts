// Source: docklet-next/issues/177
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original keyboard row ownership shared by container and stack logs. M20 #360: localised labels, neutral notation and keyboard log navigation.

import { useEffect, useId, useState, type KeyboardEvent, type RefObject } from "react";
import type { LogLine } from "@/domain/model";

export function useLogKeyboard(lines: LogLine[], follow: boolean, setFollow: (value: boolean) => void, scrollRef: RefObject<HTMLDivElement | null>) {
  const prefix = useId();
  const [selected, setSelected] = useState<string>();
  // Eviction/filtering immediately clears the descendant; it never names a missing element.
  const active = lines.some((line) => line.id === selected) ? selected : undefined;
  if (selected !== undefined && active === undefined) setSelected(undefined);
  const rowId = (id: string) => `${prefix}-${id}`;
  useEffect(() => {
    if (follow && scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    else if (active) document.getElementById(`${prefix}-${active}`)?.scrollIntoView?.({ block: "nearest" });
  }, [lines, follow, active, prefix, scrollRef]);
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!["ArrowDown", "ArrowUp", "Home", "End", "Escape"].includes(event.key)) return;
    event.preventDefault();
    if (event.key === "Escape" || event.key === "End") {
      setSelected(undefined);
      if (event.key === "End") { setFollow(true); if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight; }
      return;
    }
    const index = lines.findIndex((line) => line.id === active);
    const next = event.key === "Home" ? 0 : event.key === "ArrowDown" ? Math.min(lines.length - 1, index + 1) : index < 0 ? lines.length - 1 : Math.max(0, index - 1);
    setSelected(lines[next]?.id); setFollow(false);
  };
  return { active, hintId: `${prefix}-hint`, region: { role: "listbox" as const, tabIndex: 0, "aria-activedescendant": active ? rowId(active) : undefined, "aria-describedby": `${prefix}-hint`, onKeyDown }, row: (id: string) => ({ id: rowId(id), role: "option" as const, "aria-selected": id === active }) };
}
