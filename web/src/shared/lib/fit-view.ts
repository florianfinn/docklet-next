/**
 * Views of an overview list from widest to narrowest: the full table, a
 * condensed table (secondary columns merged into fewer cells) and stacked
 * cards. Nothing is hidden in any of them; only the arrangement changes.
 */
export type FitView = "table" | "condensed" | "cards";
export const FIT_VIEWS: readonly FitView[] = ["table", "condensed", "cards"];

export interface FitState {
  view: FitView;
  /** Measured content width of a wider view when it last overflowed; 0 = not measured yet. */
  need: Partial<Record<FitView, number>>;
}

export const INITIAL_FIT: FitState = { view: "table", need: {} };

/**
 * One measurement step. A view that overflows its container records the width
 * it needed and gives way to the next narrower view; a narrower view returns
 * to the widest view whose recorded need fits again. Using the recorded width
 * (instead of the narrower view's own width) prevents flipping back and forth.
 */
export function nextFit(state: FitState, containerWidth: number, contentWidth: number, views: readonly FitView[] = FIT_VIEWS): FitState {
  if (containerWidth <= 0 || contentWidth <= 0) return state;
  const i = Math.max(0, views.indexOf(state.view));
  if (contentWidth > containerWidth + 1 && i < views.length - 1) {
    return { view: views[i + 1]!, need: { ...state.need, [state.view]: contentWidth } };
  }
  for (let j = 0; j < i; j++) {
    const need = state.need[views[j]!];
    if (need !== undefined && containerWidth >= need) return { ...state, view: views[j]! };
  }
  return state;
}
