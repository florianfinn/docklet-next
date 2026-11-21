// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #555 tests: Compose log frames read as lines in every output view.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import type { ActionRun, RunOutputLine } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { collapseOutput, presentOutputText } from "./output-collapse";
import { outputText } from "./run-output-text";
import { RunOutput } from "./RunOutput";

// Frames exactly as the backend writer records them (writer_test.go), for runs stored before and after M35.
const created = "{\"log\":\"Container web-1  Created\"}";
const error = "{\"error\":\"Error response from daemon: conflict\"}";

it("decodes only a single-key string log frame, once", () => {
  expect(presentOutputText(created)).toBe("Container web-1  Created");
  expect(presentOutputText("{\"log\":\"line\\n\"}")).toBe("line");
  expect(presentOutputText("{\"log\":\"{\\\"error\\\":\\\"example\\\"}\"}")).toBe("{\"error\":\"example\"}");
  expect(presentOutputText("{\"log\":\"{\\\"log\\\":\\\"inner\\\"}\"}")).toBe("{\"log\":\"inner\"}");
  for (const verbatim of [error, "{\"log\":\"x\",\"stream\":\"stdout\"}", "{\"log\":42}", "{\"log\":null}", "{\"log\":", "[\"log\"]", "plain text", "{}"])
    expect(presentOutputText(verbatim)).toBe(verbatim);
});

function line(text: string, at = "2026-11-12T12:00:00.000Z"): RunOutputLine { return { at, stream: "stdout", text }; }

it("folding and the plain-text export use the decoded line", () => {
  const output = [line("{\"log\":\" 0123456789ab Downloading\"}", "2026-11-12T12:00:00.000Z"), line("{\"log\":\" 0123456789ab Pull complete\"}", "2026-11-12T12:00:01.000Z"), line(error, "2026-11-12T12:00:02.000Z")];
  const rows = collapseOutput(output);
  expect(rows.map((row) => row.line.text)).toEqual([" 0123456789ab Pull complete", error]);
  const text = outputText({ title: "Deploy", output });
  expect(text.includes("{\"log\"")).toBe(false);
  expect(text.includes(error)).toBe(true);
});

it("a stored run shows Compose lines without the frame", () => {
  const at = "2026-11-12T12:00:00Z";
  const run = renderRun(mapActivityDetail({ activity: { id: "example-run", type: "project_deploy", environmentId: "direct", status: "success", startedAt: at, createdAt: at, finishedAt: at }, messages: [], output: [{ timestamp: at, stream: "stdout", text: created }, { timestamp: at, stream: "stderr", text: error }], stepTrace: [], outputTruncated: false } as never), "de") as ActionRun;
  renderWithApp(<RunOutput run={run} defaultOpen />);
  expect(screen.queryByText("Container web-1  Created", { normalizer: (s) => s }) !== null).toBe(true);
  expect(document.body.textContent?.includes("{\"log\"")).toBe(false);
  expect(document.body.textContent?.includes(error)).toBe(true);
});
