// Source: docklet-next/issues/615
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original compact update panel tests (M40 break-map rows 6 to 8): agent anchors, manual-command disclosure, hub-host hints and empty orphan sections.

import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntlProvider, createTranslator } from "use-intl";
import { MemoryRouter } from "react-router";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { catalogs } from "@/app/i18n/catalog";
import type { SelfUpdateCheck } from "@/domain/self-update";
import type { SystemHints, UpdatePoint } from "@/domain/stack-updates";
import { LiveUpdatesSelfUpdate } from "./LiveUpdatesSelfUpdate";
import { HostUpdateSettings } from "./LiveUpdatesSection";

const t = createTranslator({ locale: "en-US", messages: catalogs["en-US"] });
const manual = ["docker compose pull", "docker compose up -d"];
const answer = (eligibility: Partial<SelfUpdateCheck["eligibility"]>, extra: Partial<SelfUpdateCheck> = {}): SelfUpdateCheck => ({ version: "1.5.0", eligibility: { state: "available", candidate: { version: "1.6.0" }, manual, ...eligibility }, ...extra });
const cleanups: (() => void)[] = [];
// Query observers are notified on a later task; wait for the rendered phase, not only the refetch.
const phase = async (client: QueryClient, available: boolean) => {
  await act(async () => { await client.refetchQueries(); });
  await waitFor(() => expect(screen.queryByText(t("common.versionBadge.available", { version: "1.6.0" })) !== null).toBe(available));
};
afterEach(() => { cleanup(); cleanups.splice(0).forEach((f) => f()); vi.restoreAllMocks(); });
function adapterWith(overrides: Partial<DockletAdapter>): DockletAdapter {
  return { ...createDisconnectedAdapter(), kind: "http", getCapability: vi.fn(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true })), ...overrides };
}
function mount(ui: React.ReactNode, adapter: DockletAdapter, entry = "/settings/updates") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  return { client, ...render(<QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter initialEntries={[entry]}>{ui}</MemoryRouter></IntlProvider></AdapterContext.Provider></QueryClientProvider>) };
}

describe("row 6: agent anchors", () => {
  it("focuses only the addressed agent's panel", async () => {
    const scrolled: Element[] = [], focused: Element[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (this: HTMLElement) { scrolled.push(this); });
    const focus = HTMLElement.prototype.focus;
    vi.spyOn(HTMLElement.prototype, "focus").mockImplementation(function (this: HTMLElement, options?: FocusOptions) { focused.push(this); focus.call(this, options); });
    const adapter = adapterWith({ getSelfUpdate: vi.fn(async () => answer({})) });
    mount(<><LiveUpdatesSelfUpdate hostId="a" name="Alpha" /><LiveUpdatesSelfUpdate hostId="b" name="Beta" /></>, adapter, "/settings/updates#agent-update-b");
    const beta = await screen.findByRole("button", { name: t("settings.updates.selfUpdate.agentOn", { name: "Beta" }) });
    await waitFor(() => expect(document.activeElement === beta).toBe(true));
    const alpha = document.getElementById("agent-update-a")!;
    expect(scrolled.some((e) => alpha.contains(e))).toBe(false);
    expect(focused.some((e) => alpha.contains(e))).toBe(false);
    expect(document.getElementById("agent-update-b")!.contains(scrolled[0]!)).toBe(true);
  });
  it("falls back to the heading when the agent cannot be updated", async () => {
    const adapter = adapterWith({ getSelfUpdate: vi.fn(async () => answer({ state: "current", candidate: undefined })) });
    mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapter, "/settings/updates#agent-update-b");
    await screen.findByText(t("common.versionBadge.current", { version: "1.5.0" }));
    await waitFor(() => expect(document.activeElement?.tagName).toBe("H4"));
    expect(document.activeElement?.closest("#agent-update-b") !== null).toBe(true);
  });
});

describe("row 6b: the anchor target stays in view while content above loads (candidate H2, R1)", () => {
  class FakeResizeObserver { static all: FakeResizeObserver[] = []; cb: () => void; constructor(cb: () => void) { this.cb = cb; FakeResizeObserver.all.push(this); } observe() {} disconnect() { FakeResizeObserver.all = FakeResizeObserver.all.filter((o) => o !== this); } }
  const shift = () => act(() => { FakeResizeObserver.all.forEach((o) => o.cb()); });
  it.each([
    ["agent", "b", "#agent-update-b", () => screen.findByRole("button", { name: t("settings.updates.selfUpdate.agentOn", { name: "Beta" }) })],
    ["hub", "0", "#hub-update", () => screen.findByRole("button", { name: t("settings.updates.selfUpdate.hub") })],
  ] as const)("%s: a later layout shift scrolls the focused target back until the user acts", async (_, hostId, hash, find) => {
    FakeResizeObserver.all = []; vi.stubGlobal("ResizeObserver", FakeResizeObserver); cleanups.push(() => vi.unstubAllGlobals());
    const scrolled: Element[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (this: HTMLElement) { scrolled.push(this); });
    mount(<LiveUpdatesSelfUpdate hostId={hostId} name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => answer({})) }), `/settings/updates${hash}`);
    const target = await find();
    await waitFor(() => expect(document.activeElement === target).toBe(true));
    const before = scrolled.filter((e) => e === target).length;
    let top = 100; vi.spyOn(target, "getBoundingClientRect").mockImplementation(() => ({ top } as DOMRect));
    top = 900; shift();
    expect(scrolled.filter((e) => e === target).length).toBe(before + 1);
    window.dispatchEvent(new Event("wheel"));
    top = 1500; shift();
    expect(scrolled.filter((e) => e === target).length).toBe(before + 1);
  });
  it("stops for good once focus has left the target, even if it returns before the next resize (accfix-1 B1)", async () => {
    FakeResizeObserver.all = []; vi.stubGlobal("ResizeObserver", FakeResizeObserver); cleanups.push(() => vi.unstubAllGlobals());
    const scrolled: Element[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (this: HTMLElement) { scrolled.push(this); });
    mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => answer({})) }), "/settings/updates#agent-update-b");
    const target = await screen.findByRole("button", { name: t("settings.updates.selfUpdate.agentOn", { name: "Beta" }) });
    await waitFor(() => expect(document.activeElement === target).toBe(true));
    const before = scrolled.filter((e) => e === target).length;
    let top = 100; vi.spyOn(target, "getBoundingClientRect").mockImplementation(() => ({ top } as DOMRect));
    act(() => { screen.getByRole("button", { name: t("settings.updates.selfUpdate.checkFor", { name: "Beta" }) }).focus(); target.focus(); });
    top = 900; shift();
    expect(scrolled.filter((e) => e === target).length).toBe(before);
  });
});

describe("row 7: manual-command disclosure", () => {
  const trigger = () => screen.queryByRole("button", { name: t("settings.updates.selfUpdate.manualCommands", { count: 2 }) });
  it.each([
    ["available", answer({}), false],
    ["current", answer({ state: "current", candidate: undefined }), false],
    ["not-eligible", answer({ state: "not-eligible", code: "update.self-custom-image" }), true],
    ["unavailable", answer({ state: "unavailable", code: "agent.unavailable", candidate: undefined }), true],
    ["busy host", answer({ state: "not-eligible", code: "host.busy" }), false],
    ["running operation", answer({ state: "not-eligible", code: "update.self-failed" }, { operation: { opId: "op", phase: "helper-started", targetVersion: "1.6.0", helperRunning: true } }), false],
  ] as const)("%s opens by default: %s", async (_, value, open) => {
    mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => value) }));
    await waitFor(() => expect(trigger() !== null).toBe(true));
    expect(trigger()!.getAttribute("aria-expanded")).toBe(String(open));
    expect(screen.queryByText(manual[0]!) !== null).toBe(open);
  });
  it("renders no disclosure without commands", async () => {
    mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => answer({ state: "not-eligible", manual: [] })) }));
    await screen.findByText("1.5.0");
    expect(screen.queryByRole("button", { name: /manual/i })).toBe(null);
  });
  it("keeps a toggle across a refresh of the same class and resets it when the class changes", async () => {
    let value = answer({});
    const f = mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => value) }));
    await waitFor(() => expect(trigger() !== null).toBe(true));
    trigger()!.focus(); await userEvent.setup().keyboard("{Enter}");
    expect(screen.queryByText(manual[0]!) !== null).toBe(true);
    await phase(f.client, true);
    expect(trigger()!.getAttribute("aria-expanded")).toBe("true");
    value = answer({ state: "not-eligible", code: "update.self-custom-image" });
    await phase(f.client, false);
    expect(trigger()!.getAttribute("aria-expanded")).toBe("true");
    await userEvent.setup().click(trigger()!);
    expect(trigger()!.getAttribute("aria-expanded")).toBe("false");
    value = answer({});
    await phase(f.client, true);
    expect(trigger()!.getAttribute("aria-expanded")).toBe("false");
  });
});

describe("row 7: disclosure reset across class changes (S3-R1-B1)", () => {
  const trigger = () => screen.queryByRole("button", { name: t("settings.updates.selfUpdate.manualCommands", { count: 2 }) });
  it.each([
    ["a closed not-eligible panel reopens after an available phase", answer({ state: "not-eligible", code: "update.self-custom-image" }), answer({}), "true"],
    ["an opened available panel closes again after a not-eligible phase", answer({}), answer({ state: "not-eligible", code: "update.self-custom-image" }), "false"],
  ] as const)("%s", async (_, start, middle, expected) => {
    let value: SelfUpdateCheck = start;
    const f = mount(<LiveUpdatesSelfUpdate hostId="b" name="Beta" />, adapterWith({ getSelfUpdate: vi.fn(async () => value) }));
    await waitFor(() => expect(trigger() !== null).toBe(true));
    await userEvent.setup().click(trigger()!);
    const startAvailable = start.eligibility.state === "available";
    value = middle; await phase(f.client, !startAvailable);
    value = start; await phase(f.client, startAvailable);
    expect(trigger()!.getAttribute("aria-expanded")).toBe(expected);
  });
});

describe("row 8: hints and orphaned restore points", () => {
  const digest = "a".repeat(64);
  const hints: SystemHints = { hubVersion: "1.6.0", images: [
    { kind: "agent", reference: "ghcr.io/example/agent:1.5.0", state: "unknown", requiresHubUpdate: false, manualOnly: true, code: "update.system-version-unknown" },
    { kind: "tools", reference: `ghcr.io/example/tools:1.0.0@sha256:${digest}`, compatibleReference: `ghcr.io/example/tools:1.0.0@sha256:${digest}`, currentVersion: "1.0.0", state: "current", requiresHubUpdate: false, manualOnly: true },
  ] };
  const point = (orphaned: boolean): UpdatePoint => ({ id: "p1", registrationId: "r", targetId: "stack-a", targetIdentity: "id-a", sourceRunId: "run", createdAt: "2026-11-25T10:00:00Z", sourceState: "settled", available: true, orphaned, volumeDataIncluded: false, protectionReason: null, holdState: "none", failedAttempt: false, images: [] });
  const settings = (hostId: string, points: () => Promise<UpdatePoint[]>) => mount(<HostUpdateSettings hostId={hostId} name="Example host" />, adapterWith({
    getSelfUpdate: vi.fn(async () => answer({ state: "current", candidate: undefined })), getSystemImageHints: vi.fn(async () => hints), listHostRestorePoints: vi.fn(points),
  }));
  const agentLabel = t("hosts.card.systemComponent.agent");
  it("drops the agent hint on the hub host and shows each reference once, shortened and copyable", async () => {
    settings("0", async () => []);
    const list = await screen.findByRole("list");
    expect(within(list).queryByText(agentLabel)).toBe(null);
    expect(screen.queryByText(t("common.stackUpdates.hubVersion", { version: "1.6.0" }))).toBe(null);
    const refs = within(list).getAllByText((_, e) => e?.tagName === "CODE");
    expect(refs.length).toBe(1);
    expect(refs[0]!.textContent).toBe(`ghcr.io/example/tools:1.0.0@sha256:${digest.slice(0, 12)}…`);
    const clipboard = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    await userEvent.setup().click(within(list).getByRole("button", { name: "Copy" }));
    expect(clipboard).toHaveBeenCalledWith(hints.images[1]!.reference);
  });
  it("keeps the agent hint and its reason on agent hosts", async () => {
    settings("7", async () => []);
    const list = await screen.findByRole("list");
    expect(within(list).queryByText(agentLabel) !== null).toBe(true);
  });
  it("hides the orphan heading without entries and shows it with entries or a read error", async () => {
    const empty = settings("7", async () => [point(false)]);
    await screen.findByRole("list"); await act(async () => {});
    expect(screen.queryByText(t("common.stackUpdates.orphans"))).toBe(null);
    empty.unmount();
    const listed = settings("7", async () => [point(true)]);
    expect(await screen.findByText(t("common.stackUpdates.orphans")) !== null).toBe(true);
    listed.unmount();
    settings("7", async () => { throw new Error("boom"); });
    expect(await screen.findByText(t("common.stackUpdates.orphans")) !== null).toBe(true);
    expect(screen.getAllByRole("alert").length > 0).toBe(true);
  });
});
