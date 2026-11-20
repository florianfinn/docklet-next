// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original rendered EN/DE eligibility, confirmation and independent follow regression tests.

import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { HubUpdateOverlay } from "@/shared/components/hub-update/HubUpdateOverlay";
import { createHubUpdateSession } from "@/data/http/hub-update-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider, createTranslator } from "use-intl";
import { MemoryRouter } from "react-router";
import { AdapterContext } from "@/data/use-adapter";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import type { DockletAdapter } from "@/data/adapter";
import { catalogs } from "@/app/i18n/catalog";
import type { SelfUpdateCheck, SelfUpdateInput } from "@/domain/self-update";
import { SELF_UPDATE_CODE_KEYS, SELF_UPDATE_STATE_KEYS } from "@/data/wire-catalogue-self-update";
import { activityStore } from "@/features/activity/activity-store";
import { LiveUpdatesSelfUpdate } from "./LiveUpdatesSelfUpdate";

type Locale = keyof typeof catalogs;
const check: SelfUpdateCheck = { version: "1.0.0", eligibility: { state: "available", candidate: { version: "1.0.1" }, manual: ["docker compose --env-file .env pull", "docker compose --env-file .env up -d"] } };
const cleanups: (() => void)[] = [];
afterEach(() => { cleanup(); cleanups.splice(0).forEach(f => f()); activityStore.reset(); vi.useRealTimers(); });
function mount(locale: Locale = "en-US", value = check, options: { allowed?: boolean; hostId?: string; name?: string; overrides?: Partial<DockletAdapter>; overlay?: boolean } = {}) {
  const adapter: DockletAdapter = { ...createDisconnectedAdapter(), kind: "http",
    getCapability: vi.fn(async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: options.allowed !== false, disabledReasonCode: "access.admin-required" })),
    getSelfUpdate: vi.fn(async () => value), selfUpdate: vi.fn(async () => "update-run"), ...options.overrides };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); cleanups.push(() => client.clear());
  const result = render(<QueryClientProvider client={client}><AdapterContext.Provider value={adapter}><IntlProvider locale={locale} messages={catalogs[locale]}><MemoryRouter><LiveUpdatesSelfUpdate hostId={options.hostId ?? "0"} name={options.name} />{options.overlay && <HubUpdateOverlay />}</MemoryRouter></IntlProvider></AdapterContext.Provider></QueryClientProvider>);
  return { ...result, adapter, client, t: createTranslator({ locale, messages: catalogs[locale] }) };
}
for (const locale of ["en-US", "de"] as const) {
  it.each(Object.keys(SELF_UPDATE_STATE_KEYS) as Array<SelfUpdateCheck["eligibility"]["state"]>)(`UI-a ${locale} renders %s eligibility and copyable commands`, async state => {
    const f = mount(locale, { ...check, eligibility: { ...check.eligibility, state } });
    await waitFor(() => expect(screen.queryByText(f.t("settings.updates.selfUpdate.currentVersion", { version: "1.0.0" })) !== null).toBe(true));
    expect(screen.queryByText(f.t(SELF_UPDATE_STATE_KEYS[state])) !== null).toBe(true);
    expect((screen.getByRole("button", { name: f.t("settings.updates.selfUpdate.hub") }) as HTMLButtonElement).disabled).toBe(state !== "available");
    expect(screen.getByText(check.eligibility.manual![0]!) !== null).toBe(true);
    const user = userEvent.setup(); const clipboard = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    await user.click(screen.getAllByRole("button", { name: locale === "de" ? "Kopieren" : "Copy" })[0]!);
    expect(clipboard).toHaveBeenCalledWith(check.eligibility.manual![0]); clipboard.mockRestore();
    expect(screen.getByText(f.t("settings.updates.selfUpdate.targetVersion", { version: "1.0.1" })) !== null).toBe(true);
  });
  it.each(Object.entries(SELF_UPDATE_CODE_KEYS))(`UI-a ${locale} renders reason %s with disabled action`, async (code, key) => {
    const f = mount(locale, { ...check, eligibility: { ...check.eligibility, state: "not-eligible", code: code as NonNullable<SelfUpdateCheck["eligibility"]["code"]> } });
    await waitFor(() => expect(screen.queryByText(f.t(key)) !== null).toBe(true));
    expect((screen.getByRole("button", { name: f.t("settings.updates.selfUpdate.hub") }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(check.eligibility.manual![1]!) !== null).toBe(true);
  });
  it(`UI-c ${locale} hides the update and check actions for non-admins`, async () => {
    const f = mount(locale, check, { allowed: false });
    await waitFor(() => expect(f.adapter.getCapability).toHaveBeenCalledWith("selfUpdate", { kind: "hub", id: "0" }));
    await act(async () => {});
    expect(screen.queryAllByRole("button").length).toBe(0);
    expect(f.adapter.getSelfUpdate).not.toHaveBeenCalled(); expect(f.adapter.selfUpdate).not.toHaveBeenCalled();
  });
  it(`confirms the ${locale} hub target, links backup advice and exposes the existing activity view`, async () => {
    const f = mount(locale); const user = userEvent.setup();
    const button = await screen.findByRole("button", { name: f.t("settings.updates.selfUpdate.hub") });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await user.click(button);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("link", { name: f.t("settings.updates.selfUpdate.backupLink") }).getAttribute("href")).toBe("/hub-backups");
    expect(within(dialog).getByText(f.t("settings.updates.selfUpdate.noRollback")) !== null).toBe(true);
    expect(f.adapter.selfUpdate).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: f.t("settings.updates.selfUpdate.confirm") }));
    await waitFor(() => expect(f.adapter.selfUpdate).toHaveBeenCalledWith("0", { targetVersion: "1.0.1" }));
    await user.click(await screen.findByRole("button", { name: f.t("settings.updates.selfUpdate.viewRun") }));
    expect(activityStore.get()).toEqual({ open: true, tab: "actions", focusRunId: "update-run" });
  });
}
it("uses the agent capability and target without hub backup advice", async () => {
  const f = mount("en-US", check, { hostId: "example" }); const user = userEvent.setup();
  const button = await screen.findByRole("button", { name: "Update agent" }); await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await user.click(button);
  expect(screen.queryByRole("link", { name: "Open Hub backups" }) !== null).toBe(false);
  await user.click(screen.getByRole("button", { name: "Start update" }));
  await waitFor(() => expect(f.adapter.selfUpdate).toHaveBeenCalledWith("example", { targetVersion: "1.0.1" }));
  expect(f.adapter.getCapability).toHaveBeenCalledWith("selfUpdate", { kind: "host", id: "example" });
});
it("explains a changed target while confirmation is open and sends no stale start", async () => {
  const f = mount(); const user = userEvent.setup();
  const button = await screen.findByRole("button", { name: "Update Docklet" }); await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await user.click(button);
  await act(async () => { f.client.setQueryData(["settings", "self-update", "0"], { ...check, eligibility: { ...check.eligibility, candidate: { version: "1.0.2" } } }); });
  const dialog = screen.getByRole("dialog");
  await waitFor(() => expect(within(dialog).queryByText("Update availability changed. Close this dialog and check again.") !== null).toBe(true));
  const confirm = within(dialog).getByRole("button", { name: "Start update" }); expect((confirm as HTMLButtonElement).disabled).toBe(true);
  await user.click(confirm); expect(f.adapter.selfUpdate).not.toHaveBeenCalled();
});

it("names the host in agent controls and the confirmation", async () => {
  mount("en-US", check, { hostId: "example", name: "Example host" }); const user = userEvent.setup();
  const button = await screen.findByRole("button", { name: "Update agent on Example host" });
  expect(button.textContent === "Update agent").toBe(true);
  expect(screen.getByRole("button", { name: "Check again for Example host" }) !== null).toBe(true);
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await user.click(button);
  expect(within(screen.getByRole("dialog")).getByRole("heading", { name: "Update agent on Example host" }) !== null).toBe(true);
});

it("gives two host cards distinct region names", async () => {
  mount("en-US", check, { hostId: "example", name: "Example host" });
  mount("en-US", check, { hostId: "second", name: "Second host" });
  expect(!!await screen.findByRole("region", { name: "Update agent on Example host" })).toBe(true);
  expect(!!screen.getByRole("region", { name: "Update agent on Second host" })).toBe(true);
});

it("checks again on its own only while an operation is unsettled", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const settled = mount("en-US", { ...check, operation: { opId: "op", phase: "settled", outcome: "healthy", targetVersion: "1.0.1", helperRunning: false } });
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  const before = vi.mocked(settled.adapter.getSelfUpdate).mock.calls.length;
  await act(async () => { await vi.advanceTimersByTimeAsync(25_000); });
  expect(vi.mocked(settled.adapter.getSelfUpdate).mock.calls.length).toBe(before);
  cleanup();
  const running = mount("en-US", { ...check, operation: { opId: "op", phase: "helper-started", targetVersion: "1.0.1", helperRunning: true } });
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  const start = vi.mocked(running.adapter.getSelfUpdate).mock.calls.length;
  await act(async () => { await vi.advanceTimersByTimeAsync(25_000); });
  expect(vi.mocked(running.adapter.getSelfUpdate).mock.calls.length > start).toBe(true);
});


it("B19 confirmation becomes preparing immediately while intake has no response", async () => {
  let accept!: (value: { accepted: boolean; opId: string; runId: string; phase: "reserved" }) => void;
  const session = createHubUpdateSession({ baseUrl: "/api", client: { fetch: vi.fn(), token: async () => "example" }, publicFetch: vi.fn(), readRun: vi.fn() }); cleanups.push(session.clear);
  const f = mount("en-US", check, { overlay: true, overrides: { hubUpdateSession: session, selfUpdate: vi.fn((_host: string, input: SelfUpdateInput) => session.start(input.targetVersion!, () => new Promise(resolve => { accept = resolve; }))) } });
  const user = userEvent.setup(); const button = await screen.findByRole("button", { name: "Update Docklet" });
  await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await user.click(button);
  await user.click(screen.getByRole("button", { name: "Start update" }));
  expect(session.get().state.phase).toBe("starting");
  expect(document.querySelector("[data-hub-update-overlay]")?.textContent).toContain("Preparing update");
  expect(f.adapter.selfUpdate).toHaveBeenCalledTimes(1);
  await act(async () => { session.clear(); accept({ accepted: true, opId: "operation", runId: "update-run", phase: "reserved" }); });
});
