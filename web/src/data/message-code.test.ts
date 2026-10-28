// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource inventory contract tests.

import { expect, it, vi } from "vitest";
import { renderMessage } from "./message-code";
import { mapActivityDetail } from "./http/map-dto";
import { enBackend } from "./messages/en";
import { deBackend } from "./messages/de";
import { reason } from "./http/update-validation";

it("resolves feature-disabled through the declared reason and EN/DE message lookup", () => {
  const code = reason("feature-disabled");
  expect(code).toBe("feature-disabled");
  const message = { code: `images.errors.${code!.replaceAll(".", "_")}`, params: {}, sourceText: "synthetic diagnostic" };
  for (const [locale, catalogue] of [["en-US", enBackend], ["de", deBackend]] as const) {
    expect(renderMessage(message, locale)).toBe(catalogue.errors["feature-disabled"]);
  }
});

it.each(["de", "en-US"] as const)("renders M5 environment and activity codes in %s without key or source fallback", (locale) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const expected = locale === "de" ? ["Host erstellt", "Host aktualisiert", "Host entfernt", "Host verbunden", "Host getrennt", "Agent-Token erneuert"] : ["Host created", "Host updated", "Host removed", "Host connected", "Host disconnected", "Agent token regenerated"];
    const codes = ["create", "update", "delete", "connect", "disconnect", "api_key.regenerated"];
    expect(codes.map((code) => renderMessage({ code: `environment.${code}`, params: {}, sourceText: "source fallback" }, locale))).toEqual(expected);
    expect(renderMessage({ code: "activity.message", params: { message: "Operation detail" }, sourceText: "source fallback" }, locale)).toBe("Operation detail");
    const run = mapActivityDetail({ activity: { id: "run", type: "health", environmentId: "first", status: "running", startedAt: "2026-10-17T12:00:00Z", createdAt: "2026-10-17T12:00:00Z" }, messages: [{ id: "message", activityId: "run", code: "activity.message", message: "Wire message", level: "info", createdAt: "2026-10-17T12:00:00Z" }], output: [], stepTrace: [], outputTruncated: false });
    expect(run.log.map((message) => renderMessage(message, locale))).toContain("Wire message");
    expect(renderMessage({ code: "future.event", params: {}, sourceText: "Future event from agent" }, locale)).toBe("Future event from agent");
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});
