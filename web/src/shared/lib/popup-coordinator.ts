import { useCallback, useEffect, useId, useRef, useState } from "react";

import { createStore } from "./store";

/**
 * Keeps at most one interactive info popup (update, services, origin hover
 * cards) open. Opening another popup, a route or tab change, or opening a
 * sheet closes the active one. Toasts and dialogs are not coordinated here.
 */
const popupStore = createStore<{ active: string | null; epoch: number }>({ active: null, epoch: 0 });

export function closeAllPopups(): void {
  popupStore.set((s) => (s.active === null ? s : { active: null, epoch: s.epoch + 1 }));
}

export function activePopupId(): string | null {
  return popupStore.get().active;
}

/**
 * Open state for one exclusive popup. `onForcedClose` runs when another popup
 * or a context change closes this one, so callers can skip focus restoration.
 */
export function useExclusivePopup(onForcedClose?: () => void): [boolean, (open: boolean) => void] {
  const id = useId();
  const [open, setOpenState] = useState(false);
  const openRef = useRef(false);
  const forced = useRef(onForcedClose);

  useEffect(() => {
    forced.current = onForcedClose;
  });

  useEffect(
    () =>
      popupStore.subscribe(() => {
        if (!openRef.current || popupStore.get().active === id) return;
        openRef.current = false;
        forced.current?.();
        setOpenState(false);
      }),
    [id],
  );

  useEffect(
    () => () => {
      if (popupStore.get().active === id) popupStore.set((s) => ({ ...s, active: null }));
    },
    [id],
  );

  const setOpen = useCallback(
    (next: boolean) => {
      openRef.current = next;
      setOpenState(next);
      popupStore.set((s) => {
        if (next) return s.active === id ? s : { ...s, active: id };
        return s.active === id ? { ...s, active: null } : s;
      });
    },
    [id],
  );

  return [open, setOpen];
}
