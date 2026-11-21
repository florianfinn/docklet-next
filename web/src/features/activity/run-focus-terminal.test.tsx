// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #555 tests: one terminal status and unknown step progress.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { RunFocusBody } from "./RunFocus";

const at = "2026-11-12T12:00:00Z";
function finished(status: "succeeded" | "failed") {
  const wire = status === "succeeded" ? "success" : "failed";
  return renderRun(mapActivityDetail({ activity: { id: "example-run", type: "project_redeploy", environmentId: "direct", status: wire, startedAt: at, createdAt: at, finishedAt: at }, messages: [], output: [], stepTrace: [], outputTruncated: false } as never), "de");
}

it("a finished run names its result once and shows unknown progress as a dash", () => {
  for (const status of ["succeeded", "failed"] as const) {
    const view = renderWithApp(<RunFocusBody run={finished(status)} now={0} />);
    const result = catalogs.de.activity.dialog.result[status];
    expect(document.body.textContent?.split(result).length).toBeLessThanOrEqual(2);
    expect(document.body.textContent?.includes("0/0")).toBe(false);
    const progress = screen.getByText(catalogs.de.activity.dialog.progress).nextElementSibling;
    expect(progress?.textContent).toBe("–");
    view.unmount();
  }
});
