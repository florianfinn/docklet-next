// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original service widget transport mapping tests from the hub response shapes, issues #332 and #333.

import { describe, expect, it } from "vitest";
import { AdapterOutcomeError } from "../capability";
import { createHttpServiceWidgets } from "./service-widgets";

function client(reply: (url: string, init?: RequestInit) => Response) {
  const calls: { url: string; init?: RequestInit }[] = [];
  return { calls, client: { token: async () => "token", fetch: (async (input: RequestInfo | URL, init?: RequestInit) => { calls.push({ url: String(input), init }); return reply(String(input), init); }) as typeof fetch } };
}
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

describe("service widget transport", () => {
  it("lists summaries with the linked stack as inventory id and skips unknown kinds", async () => {
    const { client: c } = client(() => json([
      { kind: "minecraft", configured: true, enabled: true, openUrl: "", stack: { environmentId: "host-1", projectId: "craft" } },
      { kind: "jellyfin", configured: true, enabled: true, openUrl: "" },
    ]));
    expect(await createHttpServiceWidgets("/api", c).listServiceWidgets()).toEqual([{ kind: "minecraft", configured: true, enabled: true, openUrl: "", stackId: "host-1~craft" }]);
  });

  it("maps stale data, unknown error codes and the offered certificate of a test", async () => {
    const { calls, client: c } = client((url) => url.endsWith("/test")
      ? json({ kind: "plex", stale: false, ageSeconds: 0, error: { code: "widget.tls-failed", certificate: { hostPort: "plex.example.com:32400", sha256: "ab".repeat(32) } } })
      : json({ kind: "plex", stale: true, ageSeconds: 90, error: { code: "widget.future-code" }, plex: { streams: 1, transcodes: 0, sessions: null } }));
    const api = createHttpServiceWidgets("/api", c);
    expect(await api.getServiceWidget("plex")).toEqual({ kind: "plex", stale: true, ageSeconds: 90, error: { code: "widget.bad-response" }, plex: { streams: 1, transcodes: 0, sessions: [] } });
    const tested = await api.testServiceWidget("plex", { enabled: true, address: " https://plex.example.com:32400 ", credentialAction: "keep", credential: "ignored", openUrl: "", stackId: "host-1~media" });
    expect(tested.error).toEqual({ code: "widget.tls-failed", certificate: { hostPort: "plex.example.com:32400", sha256: "ab".repeat(32) } });
    expect(calls[1]!.url).toBe("/api/service-widgets/plex/test");
    expect(JSON.parse(String(calls[1]!.init?.body))).toEqual({ enabled: true, address: "https://plex.example.com:32400", openUrl: "", credentialAction: "keep", stack: { environmentId: "host-1", projectId: "media" } });
  });

  it("sends a replaced credential only with replace and keeps the typed problem code of a refused save", async () => {
    const { calls, client: c } = client((_, init) => JSON.parse(String(init?.body)).credentialAction === "replace"
      ? json({ kind: "overseerr", enabled: true, address: "http://192.0.2.23:5055", hasCredential: true, openUrl: "", revision: 3, updatedAt: "2026-11-03T12:00:00Z" })
      : json({ code: "widget.invalid-config" }, 400));
    const api = createHttpServiceWidgets("/api", c);
    const saved = await api.saveServiceWidgetConfig("overseerr", { enabled: true, address: "http://192.0.2.23:5055", credentialAction: "replace", credential: "example-key", openUrl: "" });
    expect([saved.hasCredential, saved.revision, calls[0]!.init?.method, JSON.parse(String(calls[0]!.init?.body)).credential]).toEqual([true, 3, "PUT", "example-key"]);
    const error = await api.saveServiceWidgetConfig("overseerr", { enabled: true, address: "x", credentialAction: "clear", openUrl: "" }).catch((e: unknown) => e);
    expect("credential" in JSON.parse(String(calls[1]!.init?.body))).toBe(false);
    expect(error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "widget.invalid-config" && error.outcome.status === 400).toBe(true);
  });

  it("posts an Overseerr decision to the request route", async () => {
    const { calls, client: c } = client(() => json({ activityId: "a1" }));
    await createHttpServiceWidgets("/api", c).decideMediaRequest(42, "decline");
    expect([calls[0]!.url, calls[0]!.init?.method]).toEqual(["/api/service-widgets/overseerr/requests/42/decline", "POST"]);
  });
});
