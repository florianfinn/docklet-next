import type { NodeChange } from "@xyflow/react";

/**
 * Focus wanted by React Flow's own selection changes (click, Enter/Space on a
 * focused node, Escape, pane click): an id, null to clear, or undefined when
 * the changes carry no selection. The page keeps focus in the URL and passes
 * it back as `selected`, so React Flow never holds a selection of its own.
 */
export function focusFromChanges(changes: readonly NodeChange[]): string | null | undefined {
  const select = changes.filter((c) => c.type === "select");
  if (!select.length) return undefined;
  return select.find((c) => c.selected)?.id ?? null;
}
