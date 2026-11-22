// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original cold summary activity-center progress coverage.

import { waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { activity } from "@/data/http/activity-reader.test-support";
import { mapActivitySummary } from "@/data/http/map-dto";
import { renderWithApp } from "@/test/render";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";

it("shows a cold deploy summary in the activity row without priming detail", async () => {
  const run = mapActivitySummary({ ...activity("good"), step: "deploy", metadata: { deployActivity: { v: 1, label: "start", services: 2 } } });
  const adapter = createDemoAdapter();
  adapter.listActions = async () => [run];
  adapter.subscribeActions = (listener) => { listener([run]); return () => {}; };
  adapter.observeActionDetail = vi.fn(() => () => {});
  activityStore.set((state) => ({ ...state, open: true, tab: "actions", focusRunId: undefined }));
  const view = renderWithApp(<ActivityCenter />, adapter);
  try {
    await waitFor(() => expect(document.querySelector('[data-slot="deploy-progress"]')?.textContent).toBe("Bereitstellen · 2 Dienste werden gestartet"));
    expect(adapter.observeActionDetail).not.toHaveBeenCalled();
  } finally { view.unmount(); activityStore.reset(); }
});
