// Source: docklet-next/issues/616
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original host card version badge tests (M40 break-map rows 2, 5a to 5c, 9 and 10).

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntlProvider, createTranslator } from "use-intl";
import { MemoryRouter } from "react-router";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { createAppQueryClient } from "@/data/app-query-client";
import { mapHost } from "@/data/http/hosts";
import { catalogs } from "@/app/i18n/catalog";
import type { Host } from "@/domain/model";
import type { SelfUpdateCheck } from "@/domain/self-update";
import type { SystemComponent } from "@/domain/system";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { selfUpdateQueryKey } from "@/shared/components/updates/use-self-update-check";
import { LiveUpdatesSelfUpdate } from "@/features/settings/sections/LiveUpdatesSelfUpdate";
import { HostCard } from "./HostCard";
import { SystemMetadata } from "./SystemMetadata";

const t = createTranslator({ locale: "en-US", messages: catalogs["en-US"] });
const outdated: SelfUpdateCheck = { version: "1.5.0", eligibility: { state: "available", candidate: { version: "1.6.0" }, manual: ["docker compose pull", "docker compose up -d"] } };
const current: SelfUpdateCheck = { version: "1.6.0", eligibility: { state: "current", manual: [] } };
type Row = Parameters<typeof mapHost>[0];
const host = (over: Partial<Row> = {}): Host => mapHost({ id: "atlas", name: "Example", color: null, registrationRevision: 2, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false, connectedAt: "2026-11-25T10:00:00Z", ...over } as Row);
const component = (hostId: string, agentVersion = "1.5.0"): SystemComponent => ({
  hostId, role: "agent", metadataOnly: true, agentVersion, contractVersion: 23, toolsImage: "ghcr.io/example/tools:1.0.0", updateRequired: false,
  containerName: "", composeProject: "", image: { repository: "", tag: "", version: "", published: "" }, state: "unknown", installDir: "", insideProjectDir: false,
  labels: {}, ports: [], mounts: [], networks: [], restartPolicy: "", memLimitMb: 0, recentLog: [],
});
function deferred<T>() { let resolve!: (v: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; }

const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach((f) => f()); });
function setup(options: { allowed?: () => boolean; get?: DockletAdapter["getSelfUpdate"] } = {}) {
  const allowed = options.allowed ?? (() => true);
  const getCapability = vi.fn(async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: operationId === "selfUpdate" ? allowed() : false, disabledReasonCode: "access.admin-required" }));
  const getSelfUpdate = vi.fn(options.get ?? (async () => structuredClone(outdated)));
  const adapter: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http", getCapability, getSelfUpdate } as DockletAdapter;
  const client: QueryClient = createAppQueryClient(); cleanups.push(() => client.clear());
  const wrap = (ui: React.ReactNode) => <QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><TooltipProvider><MemoryRouter>{ui}</MemoryRouter></TooltipProvider></IntlProvider></AdapterContext.Provider></QueryClientProvider>;
  const card = (h: Host, system: SystemComponent | undefined = component(h.id)) => wrap(<HostCard host={h} system={system} stacks={1} containers={1} allIds={[h.id]} />);
  return { adapter, client, getCapability, getSelfUpdate, wrap, card };
}
const capabilitySettled = async (f: ReturnType<typeof setup>) => {
  await waitFor(() => expect(f.getCapability.mock.calls.some(([op]) => op === "selfUpdate")).toBe(true));
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
};
const target = () => document.querySelector("[data-slot=target-version]");

describe("host card version badges", () => {
  it.each([["direct", false], ["edge", true]] as const)("row 9: an outdated %s agent links to its own update panel", async (_, isEdge) => {
    const f = setup(), h = host({ isEdge, edgeSessionId: isEdge ? "session-a" : undefined } as Partial<Row>);
    render(f.card(h));
    const link = await screen.findByRole("link", { name: t("common.versionBadge.targetLabel", { name: "Example", version: "1.6.0" }) });
    expect(link.getAttribute("href")).toBe("/settings/updates#agent-update-atlas");
    expect(link.textContent).toBe(t("common.versionBadge.target", { version: "1.6.0" }));
    expect(document.querySelector("[data-slot=installed-version]")?.textContent).toBe("1.5.0");
    expect(f.getSelfUpdate).toHaveBeenCalledTimes(1);
  });
  it("shows the current hub version in the success tone without a link", async () => {
    const f = setup({ get: async () => current });
    render(f.card(mapHost({ id: "0", name: "Local Docker", color: null, registrationRevision: 1, apiUrl: "https://hub.example.com", enabled: true, status: "online", isEdge: false } as Row), component("0", "v1.6.0")));
    await screen.findByText(t("common.versionBadge.current", { version: "1.6.0" }));
    expect(document.querySelector("[data-slot=version-badges]")?.getAttribute("data-tone")).toBe("current");
    expect(screen.queryByRole("link")).toBe(null);
  });
  it("row 2: without the update capability the card shows the installed version only and starts no check", async () => {
    const f = setup({ allowed: () => false }), h = host();
    f.client.setQueryData(selfUpdateQueryKey(h.id, h), outdated);
    render(f.card(h));
    await capabilitySettled(f);
    expect(document.querySelector("[data-slot=installed-version]")?.textContent).toBe("1.5.0");
    expect(target()).toBe(null); expect(screen.queryByRole("link")).toBe(null);
    expect(f.getSelfUpdate).not.toHaveBeenCalled();
  });
  it("row 2: without the update capability a cold card starts no check", async () => {
    const f = setup({ allowed: () => false });
    render(f.card(host()));
    await capabilitySettled(f);
    expect(f.getSelfUpdate).not.toHaveBeenCalled();
    expect(target()).toBe(null);
  });
  it("row 2: a check held across a capability loss never shows its target", async () => {
    let allowed = true; const held = deferred<SelfUpdateCheck>();
    const f = setup({ allowed: () => allowed, get: () => held.promise }), h = host();
    render(f.card(h));
    await waitFor(() => expect(f.getSelfUpdate).toHaveBeenCalledTimes(1));
    allowed = false;
    await act(async () => { await f.client.invalidateQueries({ predicate: (q) => q.queryKey[0] !== "settings" }); });
    await waitFor(() => expect(f.getCapability.mock.calls.filter(([op]) => op === "selfUpdate").length > 1).toBe(true));
    await act(async () => { held.resolve(outdated); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(target()).toBe(null); expect(screen.queryByRole("link")).toBe(null);
  });
  it.each(["offline", "token-rejected", "pending"] as const)("row 5a: a %s host starts no check with a cold cache", async (status) => {
    const f = setup();
    render(f.card(host({ status })));
    await capabilitySettled(f);
    expect(f.getSelfUpdate).not.toHaveBeenCalled();
    expect(target()).toBe(null);
    expect(screen.getByText(t("common.versionBadge.reasons.not-checked")) !== null).toBe(true);
  });
  it("row 5b: a cached target is not shown while the host is offline", async () => {
    const f = setup(), h = host({ status: "offline" });
    f.client.setQueryData(selfUpdateQueryKey(h.id, h), outdated);
    render(f.card(h));
    await capabilitySettled(f);
    expect(target()).toBe(null);
    expect(document.querySelector("[data-slot=version-badges]")?.getAttribute("data-tone")).toBe("neutral");
  });
  it("row 5c: a late answer for a host that went offline stays hidden until the host is back", async () => {
    const held = deferred<SelfUpdateCheck>();
    const f = setup({ get: () => held.promise }), online = host(), offline = { ...online, status: "offline" as const };
    const view = render(f.card(online));
    await waitFor(() => expect(f.getSelfUpdate).toHaveBeenCalledTimes(1));
    view.rerender(f.card(offline));
    await act(async () => { held.resolve(outdated); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(target()).toBe(null);
    view.rerender(f.card(online));
    await waitFor(() => expect(target() !== null).toBe(true));
  });
  it("row 10: the card and the settings panel read one cache entry without editing it", async () => {
    const f = setup(), h = host();
    render(f.wrap(<><HostCard host={h} system={component(h.id)} stacks={1} containers={1} allIds={[h.id]} /><LiveUpdatesSelfUpdate hostId={h.id} name={h.name} host={h} /></>));
    await screen.findByRole("link", { name: t("common.versionBadge.targetLabel", { name: "Example", version: "1.6.0" }) });
    await screen.findByText(t("common.versionBadge.available", { version: "1.6.0" }));
    expect(f.getSelfUpdate).toHaveBeenCalledTimes(1);
    expect(f.client.getQueryData(selfUpdateQueryKey(h.id, h))).toEqual(structuredClone(outdated));
  });
});

it("the host settings dialog keeps the full component metadata with its version row", () => {
  render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><SystemMetadata component={component("atlas", "v1.5.0")} /></IntlProvider>);
  expect(screen.getByText(t("hosts.components.version")) !== null).toBe(true);
  expect(screen.getByText("v1.5.0") !== null).toBe(true);
});
