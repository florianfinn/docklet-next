// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original recreate step localisation test from the hub recreate journal producer shape.

import { expect, it, vi } from "vitest";

import { renderRun } from "../message-code";
import { mapActivityDetail } from "./map-dto";

type Detail = Parameters<typeof mapActivityDetail>[0];

// Shape of a running recreate run: journal steps arrive as activity.message step names.
const detail = {
  activity: { id: "run", type: "container_edit", environmentId: "host", resourceType: "container", resourceId: "old", resourceName: "app", status: "running", progress: 50, startedAt: "2026-10-31T12:00:00Z", createdAt: "2026-10-31T12:00:00Z" },
  stepTrace: [
    { name: "Renaming container", code: "activity.message", status: "success", startedAt: "2026-10-31T12:00:00Z", endedAt: "2026-10-31T12:00:01Z" },
    { name: "Creating replacement", code: "activity.message", status: "running", startedAt: "2026-10-31T12:00:01Z" },
  ],
  messages: [], output: [], outputTruncated: false,
} as unknown as Detail;

it.each([
  ["de", ["Container wird umbenannt", "Ersatz wird erstellt"]],
  ["en-US", ["Renaming container", "Creating replacement"]],
] as const)("localises recreate steps for every view in %s", (locale, labels) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    expect(renderRun(mapActivityDetail(detail), locale).steps.map((step) => step.label)).toEqual(labels);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});
