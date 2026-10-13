import { createStore } from "@/shared/lib/store";

export interface ActivityUi {
  open: boolean;
  tab: "actions" | "events";
  focusRunId?: string;
}

/** Action centre visibility; opened deliberately, never by an action itself. */
export const activityStore = createStore<ActivityUi>({ open: false, tab: "actions" });

export function openActivity(focusRunId?: string, tab: ActivityUi["tab"] = "actions"): void {
  activityStore.set({ open: true, tab, focusRunId });
}
