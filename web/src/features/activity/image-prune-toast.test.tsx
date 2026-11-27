// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Image cleanup runs stay out of run toasts for issue #661.

import { act, cleanup, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { activityFixture, at, type Activity } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";
import { Toaster } from "@/shared/ui/sonner";
import { bindPresentation, presentationStore } from "./run-presentation";
import { ActionToasts } from "./ActionToasts";

afterEach(() => { toast.dismiss(); cleanup(); presentationStore.reset(); setAppLocale("de"); });

it.each([["image_prune", false], ["image_pull", true]] as const)("a new %s run shows a run toast: %s", async (type, toasts) => {
  const f = activityFixture(); f.state.lists.good = []; f.state.lists.bad = [];
  await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const view = renderWithApp(<><ActionToasts /><Toaster /></>, f.adapter);
  await waitFor(() => expect(f.sources.length).toBe(1));
  const row: Activity = { id: `run-${type}`, environmentId: "good", type, resourceType: "image", startedAt: at, createdAt: at, status: "running", bulk: { unit: "target", planned: 2, plannedKnown: true, completed: 1, succeeded: 1, failed: 0, skipped: 0, updates: 0 } };
  // Even a run bound to a toast presentation stays out of run toasts when it is an image cleanup.
  bindPresentation(row.id, "toast"); f.state.lists.good = [row]; f.state.details[row.id] = { activity: row, stepTrace: [], messages: [], output: [], outputTruncated: false };
  await act(async () => { f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "updated", activityId: row.id, environmentId: "good", activity: row } }) } as MessageEvent); });
  const visible = () => document.querySelector('[data-slot="action-toast"], [role="dialog"]') !== null;
  if (toasts) await waitFor(() => expect(visible()).toBe(true));
  else { await new Promise((resolve) => setTimeout(resolve, 300)); expect(visible()).toBe(false); }
  view.unmount();
});
