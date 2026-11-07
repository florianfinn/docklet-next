// Source: docklet-next/issues/340
// Revision: 531ec931838ed09c311ebb29a4624f211bb3a7a1
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub event title localisation tests from persisted producer shapes (M17 acceptance review M17-AR8 to AR11). M19 web consumer integration.

import { expect, it, vi } from "vitest";

import { catalogs } from "@/app/i18n/catalog";

import { renderEvent } from "../message-code";
import { EVENT_TITLE_VARIANTS, HUB_EVENT_CODES, eventTitleKey, mapEvent } from "./map-dto";

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
    expect(HUB_EVENT_CODES.length).toBe(71);
    expect(untranslated).toEqual([]);
    // Each catalogue title belongs to an event code; none is left without a producer.
    expect(Object.keys(catalogs["en-US"].runMessages.events.labels).sort()).toEqual([...HUB_EVENT_CODES.map(eventTitleKey), ...EVENT_TITLE_VARIANTS].sort());
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

// Producers that tell phase or outcome apart under one code: the updater run, fleet upgrades and webhook actions.
it.each([
  ["system.auto_update", "Auto-update run started", { severity: "info", resourceName: "auto_updater", metadata: { phase: "start" } }, "Automatische Aktualisierung gestartet", "Automatic update started"],
  ["system.auto_update", "Auto-update: image pull nginx:1.29", { resourceName: "auto_updater", metadata: { phase: "image_pull", imageNew: "nginx:1.29" } }, "Automatische Aktualisierung: Image geladen: nginx:1.29", "Automatic update: image pulled: nginx:1.29"],
  ["system.auto_update", "Auto-update: project web", { resourceName: "auto_updater", metadata: { phase: "project", projectName: "web" } }, "Automatische Aktualisierung: Stack: web", "Automatic update: stack: web"],
  ["system.auto_update", "Auto-update run completed", { resourceName: "auto_updater", metadata: { phase: "complete" } }, "Automatische Aktualisierung abgeschlossen", "Automatic update completed"],
  ["system.upgrade", "Update all environments failed", { severity: "error", resourceName: undefined, metadata: { action: "update_all_environments" } }, "Systemaktualisierung fehlgeschlagen", "System upgrade failed"],
  ["system.upgrade", "Update all environments completed with errors", { severity: "warning", resourceName: undefined, metadata: { failed: 1 } }, "Systemaktualisierung mit Fehlern abgeschlossen", "System upgrade completed with errors"],
  ["system.upgrade", "System upgrade completed", { resourceName: undefined }, "Systemaktualisierung abgeschlossen", "System upgrade completed"],
  ["volume.prune", "Volume prune: bulk_prune", { resourceId: "", resourceName: "bulk_prune" }, "Volumes bereinigt", "Volumes pruned"],
  ["network.delete", "Network deleted: bulk_prune", { resourceId: "", resourceName: "bulk_prune", metadata: { action: "prune" } }, "Netzwerk gelöscht", "Network deleted"],
  ["image.prune", "Docker image prune: prune", { resourceId: "", resourceName: "prune" }, "Images bereinigt", "Images pruned"],
  // Real resources and users with these names keep them.
  ["volume.delete", "Volume deleted: prune", { resourceId: "prune", resourceName: "prune" }, "Volume gelöscht: prune", "Volume deleted: prune"],
  ["network.delete", "Network deleted: bulk_prune", { resourceId: "net-1", resourceName: "bulk_prune" }, "Netzwerk gelöscht: bulk_prune", "Network deleted: bulk_prune"],
  ["user.login", "User logged in: prune", { resourceName: undefined, username: "prune" }, "Benutzer angemeldet: prune", "User signed in: prune"],
  ["container.start", "Container started: auto_updater", { resourceName: "auto_updater" }, "Container gestartet: auto_updater", "Container started: auto_updater"],
  ["system.auto_update", "Auto-update: constructor", { resourceName: "auto_updater", metadata: { phase: "constructor" } }, "Automatische Aktualisierung", "Automatic update"],
] as const)("keeps the producer's phase or outcome for %s: %s", (code, title, extra, de, en) => {
  const mapped = mapEvent(event(code, title, extra));
  expect([renderEvent(mapped, "de").title, renderEvent(mapped, "en-US").title]).toEqual([de, en]);
});

it.each(["notification.send", "notification.delivery-failed"])("delivery title names the exact named target and keeps old-event fallback (%s)", (code) => {
  const named = mapEvent(event(code, "English producer title", { metadata: { provider: "ntfy", targetId: 8, targetName: "Operations room", reason: "delivery-auth" } }));
  expect(renderEvent(named, "de").title).toContain("ntfy (Operations room)");
  expect(renderEvent(named, "en-US").title).toContain("ntfy (Operations room)");
  const older = mapEvent(event(code, "English producer title", { metadata: { provider: "ntfy" } }));
  expect(renderEvent(older, "de").title.includes("Operations room")).toBe(false);
});
