// Source: docklet-next/issues/98
// Revision: fdb37e836f708bd9ff76f75075bbb3e60ae2f0b3
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original host error and connected onboarding regressions.

import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { AdapterOutcomeError, unavailable } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapHost } from "@/data/http/hosts";
import { renderWithApp } from "@/test/render";
import { HostCard } from "./HostCard";
import { HostSettingsDialog } from "./HostSettingsDialog";
import { ConnectStep } from "./onboarding/ConnectSteps";
import { freshDraft } from "./onboarding/onboarding-draft";

const host = mapHost({ id: "atlas", name: "Example", color: null, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
const draft = freshDraft({ projectDir: "/srv/projects", installDir: "/srv/agent", connection: "direct", puid: "", pgid: "", detectSocketGroup: true }, []);

afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });

describe.each(["de", "en-US"] as const)("host polish (%s)", (locale) => {
  const messages = locale === "de" ? {
    mapping: "Diese Aktion ist in der aktuellen Hub-Version nicht verfügbar.",
    capability: "Diese Aktion ist nicht verfügbar, weil ihre Fähigkeit nicht gelesen werden konnte.",
    busy: "Für diesen Host läuft bereits eine andere Aktion.",
    agent: "Der Agent ist derzeit nicht erreichbar. Prüfe die Verbindung und versuche es erneut.",
    credential: "Das Agent-Token wurde abgelehnt. Installiere das neu ausgestellte Token und versuche es erneut.",
    update: "Der Agent muss vor dieser Aktion aktualisiert werden.",
    failed: "Die Host-Aktion ist fehlgeschlagen. Bitte versuche es erneut.",
  } : {
    mapping: "This action is not available in the current hub version.",
    capability: "This action is unavailable because its capability could not be read.",
    busy: "Another action is already running for this host.",
    agent: "The agent is currently unreachable. Check its connection and try again.",
    credential: "The agent token was rejected. Install the newly issued token and try again.",
    update: "The agent needs an update before this action can run.",
    failed: "The host action failed. Please try again.",
  };
  const cases = [
    ["unmapped", unavailable("reconnectHost", undefined, "capability.mapping-unavailable"), messages.mapping],
    ["capability", unavailable("reconnectHost"), messages.capability],
    ["conflict", new AdapterOutcomeError({ kind: "conflict", code: "run.cancelling", activeRunId: "example" }), messages.busy],
    ["unreachable", new AdapterOutcomeError({ kind: "error", code: "agent.unavailable" }), messages.agent],
    ["credential", new AdapterOutcomeError({ kind: "error", code: "agent.invalid-token" }), messages.credential],
    ["update", new AdapterOutcomeError({ kind: "error", code: "agent.update-required" }), messages.update],
    ["unknown typed", new AdapterOutcomeError({ kind: "error", code: "hosts.future-code" }), messages.failed],
    ["unknown error", new Error("hosts.invalid-response"), messages.failed],
  ] as const;
  it.each(cases)("localizes a real connection-check gesture for %s", async (_name, error, message) => {
    setAppLocale(locale);
    const adapter = createDemoAdapter();
    vi.spyOn(adapter, "reconnectHost").mockRejectedValue(error);
    const shown = vi.spyOn(toast, "error").mockImplementation(() => "example");
    renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, adapter);
    await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Verbindung prüfen" : "Check connection" }));
    expect(shown).toHaveBeenCalledWith(message);
  });

  it.each([1280, 390])("shows form-level settings-save errors at width %s without invalidating the name", async (width) => {
    window.innerWidth = width;
    setAppLocale(locale);
    const adapter = createDemoAdapter();
    vi.spyOn(adapter, "updateHost").mockRejectedValue(unavailable("updateHost", undefined, "capability.mapping-unavailable"));
    renderWithApp(<HostSettingsDialog host={{ ...host, color: "#22c55e" }} component={undefined} open onOpenChange={() => {}} />, adapter);
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByRole("textbox", { name: locale === "de" ? "Anzeigename" : "Display name" });
    await userEvent.clear(input); await userEvent.type(input, "Renamed");
    await userEvent.click(within(dialog).getByRole("button", { name: locale === "de" ? "Speichern" : "Save" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(messages.mapping);
    expect(input.getAttribute("aria-invalid")).not.toBe("true");
    expect(input.getAttribute("aria-describedby") ?? "").not.toContain("error");
  });

  it("does not claim connected hosts have no workloads or need their first stack", () => {
    setAppLocale(locale);
    const view = renderWithApp(<ConnectStep draft={draft} host={host} hubUrl="http://hub.example.com" />);
    expect(view.container.textContent).not.toMatch(/No stacks are running|Es laufen noch keine Stacks|Create first stack|Ersten Stack anlegen/);
    expect(screen.getByRole("link", { name: locale === "de" ? "Stack anlegen" : "Create stack" }).getAttribute("href")).toBe("/stacks/new?host=atlas");
  });

  it.each(["direct", "edge"] as const)("uses row-specific missing values for a %s host", (connection) => {
    setAppLocale(locale);
    const view = renderWithApp(<HostCard host={{ ...host, connection, address: "", status: "offline", lastSeen: "" }} system={undefined} stacks={2} containers={3} allIds={[host.id]} />);
    const value = (label: string) => [...view.container.querySelectorAll("dt")].find((row) => row.textContent === label)?.nextElementSibling?.textContent;
    expect(value(locale === "de" ? "Adresse" : "Address")).toBe(connection === "edge"
      ? locale === "de" ? "Für Edge nicht erforderlich" : "Not required for Edge"
      : locale === "de" ? "Nicht konfiguriert" : "Not configured");
    expect(value(locale === "de" ? "Letzter Kontakt" : "Last contact")).toBe(locale === "de" ? "Noch nie" : "Never");
    expect(value(locale === "de" ? "Agent-Installation" : "Agent installation")).toBe(locale === "de" ? "Vom Agent nicht gemeldet" : "Not reported by the agent");
  });

  it("renders honest unavailable values instead of empty host fields", () => {
    setAppLocale(locale);
    const view = renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />);
    const fields = [...view.container.querySelectorAll("dl:first-of-type > dd")];
    expect(fields.length).toBeGreaterThan(5);
    expect(fields.every((field) => !!field.textContent?.trim())).toBe(true);
    expect(screen.getAllByText(locale === "de" ? "Vom Agent nicht gemeldet" : "Not reported by the agent").length).toBeGreaterThanOrEqual(4);
  });
});

it.each(["fallback", "explicit", "swatch"])("opens a valid host colour and preserves it on an unchanged %s save", async (mode) => {
  const adapter = createDemoAdapter();
  const base = { ...host, ...(mode === "explicit" ? { color: "#22c55e" } : mode === "swatch" ? { color: "#112233" } : {}) };
  const save = vi.spyOn(adapter, "updateHost").mockImplementation(async (_id, input) => ({ ...base, ...input, color: input.color ?? base.color }));
  renderWithApp(<HostSettingsDialog host={base} component={undefined} open onOpenChange={() => {}} />, adapter);
  const dialog = await screen.findByRole("dialog");
  const hex = within(dialog).getByRole("textbox", { name: /Hex/ });
  expect(hex.getAttribute("aria-invalid")).not.toBe("true");
  expect((within(dialog).getByRole("button", { name: "Speichern" }) as HTMLButtonElement).disabled).toBe(true);
  if (mode === "swatch") await userEvent.click(within(dialog).getByRole("radio", { name: "Farbe #ec4899" }));
  else {
    const name = within(dialog).getByRole("textbox", { name: "Anzeigename" });
    await userEvent.clear(name); await userEvent.type(name, "Renamed");
  }
  await userEvent.click(within(dialog).getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith(base.id, { name: mode === "swatch" ? base.name : "Renamed", color: mode === "swatch" ? "#ec4899" : base.color }));
});
