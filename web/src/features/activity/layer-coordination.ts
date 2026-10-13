import { useEffect } from "react";

/**
 * Surfaces that float above side sheets and dialogs: the toaster and the
 * action dialog. Sheets and dialogs stay open when the pointer or focus moves
 * into them, so a toast stays usable while a sheet is open.
 */
const COORDINATED = '[data-sonner-toaster], [data-layer="action-dialog"]';

export function isCoordinatedLayer(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(COORDINATED) !== null;
}

/** onInteractOutside handler for sheets and dialogs that share the screen with toasts. */
export function keepOpenForCoordinatedLayers(event: { target: EventTarget | null; preventDefault: () => void }): void {
  if (isCoordinatedLayer(event.target)) event.preventDefault();
}

/**
 * Sonner returns focus to the element that had it before the toaster, as soon
 * as focus leaves the toaster. After a click on a toast that opened the action
 * centre, that return lands outside the non-modal sheet and closes it again.
 * A pointer press on a toast therefore first blurs the outside element, so
 * there is nothing to return to; keyboard entry (Alt+T) keeps the return.
 */
export function useToasterPointerFocus(): void {
  useEffect(() => {
    const onPointerDown = (e: PointerEvent) => {
      const active = document.activeElement;
      if (!(e.target instanceof Element) || !e.target.closest("[data-sonner-toaster]")) return;
      if (active instanceof HTMLElement && active !== document.body && !active.closest("[data-sonner-toaster]")) active.blur();
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    return () => document.removeEventListener("pointerdown", onPointerDown, true);
  }, []);
}
