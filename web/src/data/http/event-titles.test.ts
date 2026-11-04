// Source: docklet-next/issues/340
// Revision: 531ec931838ed09c311ebb29a4624f211bb3a7a1
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub event title localisation tests from persisted producer shapes (M17 acceptance review M17-AR8).

import { expect, it, vi } from "vitest";

import { catalogs } from "@/app/i18n/catalog";

import { renderEvent } from "../message-code";
import { HUB_EVENT_CODES, eventTitleKey, mapEvent } from "./map-dto";

type Event = Parameters<typeof mapEvent>[0];

// Shapes of persisted hub events: the hub templates, daemon observations and identity events write English titles.
const event = (messageCode: string, title: string, extra: Record<string, unknown> = {}) => ({
  id: "event", type: messageCode, messageCode, severity: "info", title, description: title, resourceType: messageCode.split(".")[0],
  resourceName: "web", userId: "system", username: "System", environmentId: "0", metadata: {}, timestamp: "2026-11-04T12:00:00Z",
  createdAt: "2026-11-04T12:00:00Z", updatedAt: "2026-11-04T12:00:00Z", ...extra,
}) as unknown as Event;

it.each([
  ["container.start", "Container started: web", {}, "Container gestartet: web", "Container started: web"],
  ["container.die", "Docker container die: web", {}, "Container beendet: web", "Container exited: web"],
  ["network.delete", "Docker network destroy: web_default", { resourceName: "web_default" }, "Netzwerk gelöscht: web_default", "Network deleted: web_default"],
  ["user.login", "User logged in: operator", { resourceName: undefined, username: "operator" }, "Benutzer angemeldet: operator", "User signed in: operator"],
  ["environment.connect", "Edge Agent Connected", { resourceName: "edge-example" }, "Host verbunden: edge-example", "Host connected: edge-example"],
  ["system.prune", "System prune completed", { resourceName: undefined }, "Systembereinigung abgeschlossen", "System cleanup completed"],
  ["webhook.create", "Webhook created: Example hook", { resourceName: "Example hook" }, "Webhook erstellt: Example hook", "Webhook created: Example hook"],
] as const)("renders the hub event %s from the catalogue", (code, title, extra, de, en) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const mapped = mapEvent(event(code, title, extra));
    expect([renderEvent(mapped, "de").title, renderEvent(mapped, "en-US").title]).toEqual([de, en]);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

it("gives every catalogued hub event a German title instead of the hub's English text", () => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const english = "Hub generated English title";
    const untranslated = HUB_EVENT_CODES.filter((code) => renderEvent(mapEvent(event(code, english)), "de").title.includes(english));
    expect(HUB_EVENT_CODES.length).toBe(75);
    expect(untranslated).toEqual([]);
    // Each catalogue title belongs to an event code; none is left without a producer.
    expect(Object.keys(catalogs["en-US"].runMessages.events.labels).sort()).toEqual(HUB_EVENT_CODES.map(eventTitleKey).sort());
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});
