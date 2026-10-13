import type { MouseEvent } from "react";
import { useNavigate } from "react-router";

const INTERACTIVE = [
  "a[href]",
  "button",
  "input",
  "select",
  "textarea",
  "label",
  "summary",
  "[contenteditable='true']",
  "[role='button']",
  "[role='checkbox']",
  "[role='menuitem']",
  "[role='switch']",
  "[role='tab']",
  "[data-row-stop]",
].join(",");

/** True when the click started on a nested control inside the row. */
export function isNestedInteractive(target: EventTarget | null, row: HTMLElement): boolean {
  if (!(target instanceof Element)) return false;
  const hit = target.closest(INTERACTIVE);
  return !!hit && hit !== row && row.contains(hit);
}

/**
 * Click handler for rows that navigate as a whole. Nested controls, portalled
 * overlay content (React bubbles through portals) and text selection are ignored.
 */
export function useRowNavigation() {
  const navigate = useNavigate();
  return (href: string) => ({
    onClick(event: MouseEvent<HTMLElement>) {
      const row = event.currentTarget;
      if (event.defaultPrevented || event.button !== 0) return;
      if (!(event.target instanceof Node) || !row.contains(event.target)) return;
      if (isNestedInteractive(event.target, row)) return;
      if (window.getSelection()?.toString()) return;
      if (event.metaKey || event.ctrlKey) {
        window.open(href, "_blank", "noopener");
        return;
      }
      navigate(href);
    },
  });
}
