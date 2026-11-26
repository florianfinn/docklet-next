// Source: docklet-next/web/src/features/activity/ActionSteps.tsx
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original regression coverage for the indeterminate step track.

import { cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { ActionToastView } from "./ActionToastView";

const at = "2026-11-25T12:00:00Z";
// The upgrade service records agent updates without a step trace.
function agentUpdate(status: "running" | "success") {
  return renderRun(mapActivityDetail({ activity: { id: "example-agent-update", type: "agent.update", environmentId: "1", resourceType: "system", resourceId: "1", status, startedAt: at, createdAt: at, ...(status === "success" ? { finishedAt: at } : {}) }, stepTrace: [], messages: [], output: [], outputTruncated: false }), "en-US");
}
afterEach(cleanup);

it.each(["bar", "compact", "edge"] as const)("shows an indeterminate bar for a running run without steps (%s)", (design) => {
  const view = renderWithApp(<ActionToastView run={agentUpdate("running")} design={design} detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />);
  const track = view.container.querySelector<HTMLElement>('[data-slot="step-track"]');
  expect(track?.dataset.indeterminate === "true" && track.querySelector('[data-slot="step-motion"]') !== null).toBe(true);
});

it("leaves a settled run without steps without a bar", () => {
  const view = renderWithApp(<ActionToastView run={agentUpdate("success")} design="bar" detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />);
  expect(view.container.querySelector('[data-slot="step-motion"]') === null).toBe(true);
});
