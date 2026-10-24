// Source: docklet-next/issues/211
// Revision: 31a3237938f4b560d550c9f0a34ebc547fddde0d
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original abort-ignoring history fixture for consumer and identity wiring.

import { createHttpAdapter } from "./http-adapter";
import { ActivitySource, at, json } from "./activity-reader.test-support";

export function eventDemandFixture() {
  const source = new ActivitySource();
  const state = {
    rows: Array.from({ length: 792 }, (_, id) => ({ id: `event-${id}`, type: "project.deploy", messageCode: "project.deploy", severity: "success", timestamp: at, createdAt: at, title: "Example" })),
    hold: undefined as undefined | ((response: Response, index: number) => Promise<Response>),
  };
  const calls: { start: number; limit: number }[] = [];
  const adapter = createHttpAdapter({ eventSource: () => source as unknown as EventSource, fetch: async (input) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/capabilities")) return json([{ operationId: "listEvents", available: true, resourceRef: { kind: "hub", id: "0" } }]);
    if (url.pathname.startsWith("/api/auth/")) {
      const user = { id: "operator", username: "operator", requiresPasswordChange: false, locale: "en-US" };
      return json(url.pathname.endsWith("/me") ? user : url.pathname.endsWith("/login") ? { user } : {});
    }
    if (url.pathname.endsWith("/events")) {
      const start = Number(url.searchParams.get("start")); const limit = Number(url.searchParams.get("limit"));
      calls.push({ start, limit });
      const response = json(state.rows.slice(start, start + limit), 200, state.rows.length);
      return state.hold ? state.hold(response, calls.length - 1) : response;
    }
    return json([]);
  } });
  const notify = () => source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: at, eventLog: { type: "changed", timestamp: at } }), lastEventId: "" } as MessageEvent);
  return { adapter, source, state, calls, notify };
}
