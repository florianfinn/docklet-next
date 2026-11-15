// Source: docklet-next/issues/429
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original reload tests of the host restart panel through the HTTP adapter.

import { act, screen, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createHttpAdapter } from "@/data/http/http-adapter";
import type { CodedActionRun } from "@/data/message-code";
import { usePrincipal } from "@/data/use-principal";
import { ActionToasts } from "@/features/activity/ActionToasts";
import { mapHost } from "@/data/http/hosts";
import { ActivitySource, json as listJson } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";
import { HostSettingsDialog } from "./HostSettingsDialog";

type Restart = components["schemas"]["EnvironmentAgentRestartRun"];
const host = mapHost({ id: "example-host", name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
const refused: Restart = { runId: "run-refused", createdAt: "2026-11-13T10:00:00Z", deadline: "2026-11-13T10:03:00Z", state: "failed", code: "host.busy", settledAt: "2026-11-13T10:00:02Z" };
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });

// A stub hub that keeps the durable restart row across adapter instances (a browser reload).
function stubHub(latest: () => Promise<Response> | Response) {
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/agent/restarts/latest")) { fetcher.signals.push(init?.signal ?? undefined); return latest(); }
    const data = url.pathname === "/api/capabilities" ? [] : [];
    return new Response(JSON.stringify({ success: true, data, pagination: { totalItems: 0 } }), { headers: { "Content-Type": "application/json" } });
  }) as ReturnType<typeof vi.fn> & { signals: (AbortSignal | undefined)[] };
  fetcher.signals = [];
  return fetcher;
}
const answer = (run?: Restart) => new Response(JSON.stringify({ success: true, data: run ? { run } : {} }), { headers: { "Content-Type": "application/json" } });

async function openAgentPanel(fetcher: typeof fetch, tab: string, adapter = createHttpAdapter({ fetch: fetcher })) {
  const view = renderWithApp(<HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} />, adapter);
  await userEvent.click(screen.getByRole("tab", { name: tab }));
  return view;
}

// Rows 4 and 5b: after a reload the panel names the agent's busy refusal and its finish time.
it.each([
  ["en-US", "Agent container", "Restart refused: a backup or restore is running on this host.", "Finished "],
  ["de", "Agent-Container", "Neustart abgelehnt: Auf diesem Host läuft ein Backup oder eine Wiederherstellung.", "Beendet "],
] as const)("shows the stored refusal after a reload in %s", async (locale, tab, reason, finished) => {
  setAppLocale(locale);
  const fetcher = stubHub(() => answer(refused));
  const first = await openAgentPanel(fetcher as unknown as typeof fetch, tab);
  expect((await screen.findByRole("alert")).textContent).toBe(reason);
  first.unmount();
  await openAgentPanel(fetcher as unknown as typeof fetch, tab);
  expect((await screen.findByRole("alert")).textContent).toBe(reason);
  expect(screen.getByText((text) => text.startsWith(finished)) !== null).toBe(true);
  expect(fetcher.signals.length).toBe(2);
});

it("shows no restart history for a host that never restarted", async () => {
  setAppLocale("en-US");
  const fetcher = stubHub(() => answer());
  await openAgentPanel(fetcher as unknown as typeof fetch, "Agent container");
  await waitFor(() => expect(fetcher.signals.length).toBe(1));
  expect(screen.queryByRole("region", { name: "Agent restart progress" }) === null).toBe(true);
});

// Row 7: leaving the agent panel before the read answers aborts it while the app stays
// mounted; reopening reads again and shows the outcome.
it("aborts the read when the panel closes before the hub answers", async () => {
  setAppLocale("en-US");
  let release!: (response: Response) => void;
  const fetcher = stubHub(() => fetcher.signals.length === 1 ? new Promise<Response>((done) => { release = done; }) : answer(refused));
  await openAgentPanel(fetcher as unknown as typeof fetch, "Agent container");
  await waitFor(() => expect(fetcher.signals.length).toBe(1));
  await userEvent.click(screen.getByRole("tab", { name: "Name & color" }));
  await waitFor(() => expect(fetcher.signals[0]?.aborted).toBe(true));
  release(answer(refused));
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  expect((await screen.findByRole("alert")).textContent).toBe("Restart refused: a backup or restore is running on this host.");
  expect(fetcher.signals.length).toBe(2);
});

// Reopening the agent panel reads the hub again instead of reusing an earlier answer.
it("reads again when the agent panel is reopened", async () => {
  setAppLocale("en-US");
  const fetcher = stubHub(() => answer(fetcher.signals.length === 1 ? undefined : refused));
  await openAgentPanel(fetcher as unknown as typeof fetch, "Agent container");
  await waitFor(() => expect(fetcher.signals.length).toBe(1));
  await userEvent.click(screen.getByRole("tab", { name: "Name & color" }));
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  expect((await screen.findByRole("alert")).textContent).toBe("Restart refused: a backup or restore is running on this host.");
  expect(fetcher.signals.length).toBe(2);
});

const followed: Restart = { runId: "run-followed", createdAt: "2026-11-13T10:00:00Z", deadline: "2026-11-13T10:03:00Z", state: "sent" };
// A signed-in hub for the real toast consumer: `latest` answers the latest read, `detail` the follow
// reads and `activities` the host's activity history.
function appHub(latest: () => Restart, detail: () => Restart, activities: () => Promise<unknown[]> | unknown[] = () => []) {
  const rest = stubHub(() => answer(latest()));
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data, pagination: { totalItems: Array.isArray(data) ? data.length : 0 } }), { headers: { "Content-Type": "application/json" } });
    if (url.pathname === "/api/auth/me") return json({ id: "example-operator", username: "operator", requiresPasswordChange: false, locale: "en-US", isAdmin: true });
    if (url.pathname === "/api/settings/timezone") return json({ timezone: "UTC" });
    if (url.pathname === "/api/capabilities") return json(["listActions", "listHosts", "restartAgent", "reconnectHost"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (url.pathname.endsWith("/activities")) return json(await activities());
    if (/\/agent\/restarts\/(?!latest)[^/]+$/.test(url.pathname)) return json(detail());
    return (rest as unknown as typeof fetch)(input, init);
  });
  return { fetcher, rest };
}
function SignedInToasts() { return usePrincipal().data ? <><ActionToasts /><span data-testid="toasts-ready" /></> : null; }

// Row 1w: a run still active after a reload notifies once when it settles, whether or not the
// activity history arrived before the panel read it.
it.each([false, true])("notifies once for a followed restart (activity history first: %s)", async (historyFirst) => {
  setAppLocale("en-US");
  let current = followed;
  const f = appHub(() => current, () => current);
  const adapter = createHttpAdapter({ fetch: f.fetcher as unknown as typeof fetch });
  await adapter.getCurrentUser();
  if (historyFirst) await adapter.listActions();
  const shows = vi.spyOn(toast, "custom");
  const view = renderWithApp(<><SignedInToasts /><HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} /></>, adapter);
  await screen.findByTestId("toasts-ready");
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Restarting …" }).getAttribute("aria-busy")).toBe("true"));
  await userEvent.click(screen.getByRole("tab", { name: "Name & color" }));
  current = { ...followed, state: "connected", settledAt: "2026-11-13T10:00:30Z" };
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  await waitFor(() => expect(screen.getByText((text) => text.startsWith("Finished ")) !== null).toBe(true));
  expect(shows.mock.calls.filter(([, options]) => options?.id === followed.runId)).toHaveLength(1);
  view.unmount();
});

// Row 8: settled history read after a reload never reaches the action store or a toast.
it("keeps settled history silent", async () => {
  setAppLocale("en-US");
  const f = appHub(() => refused, () => refused);
  const adapter = createHttpAdapter({ fetch: f.fetcher as unknown as typeof fetch });
  await adapter.getCurrentUser();
  const snapshots: CodedActionRun[][] = [];
  const off = adapter.subscribeActions((runs) => snapshots.push(runs));
  const shows = vi.spyOn(toast, "custom");
  const view = renderWithApp(<><SignedInToasts /><HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} /></>, adapter);
  await screen.findByTestId("toasts-ready");
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  await screen.findByRole("alert");
  expect(snapshots.flat().some((run) => run.id === refused.runId)).toBe(false);
  expect(shows.mock.calls.filter(([, options]) => options?.id === refused.runId)).toHaveLength(0);
  view.unmount(); off();
});

// A later stored run outranks a different, older run observed in this session.
it("shows a newer stored run instead of an older session run", async () => {
  setAppLocale("en-US");
  let current = followed;
  const f = appHub(() => current, () => ({ ...followed, state: "connected", settledAt: "2026-11-13T10:00:30Z" }));
  const adapter = createHttpAdapter({ fetch: f.fetcher as unknown as typeof fetch });
  await adapter.getCurrentUser();
  const snapshots: CodedActionRun[][] = [];
  const off = adapter.subscribeActions((runs) => snapshots.push(runs));
  await adapter.latestAgentRestart!(host.id);
  await waitFor(() => expect(snapshots.flat().some((run) => run.id === followed.runId && run.status === "succeeded")).toBe(true));
  current = { ...refused, runId: "run-newer", createdAt: "2026-11-13T11:00:00Z", settledAt: "2026-11-13T11:00:02Z" };
  const view = await openAgentPanel(f.fetcher as unknown as typeof fetch, "Agent container", adapter);
  expect((await screen.findByRole("alert")).textContent).toBe("Restart refused: a backup or restore is running on this host.");
  view.unmount(); off();
});

// Row 1h: a restart read published before the first activity history cannot seed that history;
// older failed runs arriving afterwards stay silent while the restart still notifies.
it("keeps older failed runs silent when the restart read precedes the activity history", async () => {
  setAppLocale("en-US");
  let release!: () => void;
  const held = new Promise<void>((done) => { release = done; });
  const older = { id: "run-older", environmentId: "0", status: "failed", type: "project_redeploy", startedAt: "2026-11-13T09:00:00Z", createdAt: "2026-11-13T09:00:00Z", endedAt: "2026-11-13T09:00:05Z", resourceName: "Example app" };
  const f = appHub(() => followed, () => followed, async () => { await held; return [older]; });
  const adapter = createHttpAdapter({ fetch: f.fetcher as unknown as typeof fetch });
  await adapter.getCurrentUser();
  const snapshots: CodedActionRun[][] = [];
  const off = adapter.subscribeActions((runs) => snapshots.push(runs));
  const shows = vi.spyOn(toast, "custom");
  const view = renderWithApp(<><SignedInToasts /><HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} /></>, adapter);
  await screen.findByTestId("toasts-ready");
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  await waitFor(() => expect(snapshots.flat().some((run) => run.id === followed.runId)).toBe(true));
  expect(snapshots.flat().some((run) => run.id === older.id)).toBe(false);
  release();
  await waitFor(() => expect(snapshots.flat().some((run) => run.id === older.id)).toBe(true));
  expect(shows.mock.calls.filter(([, options]) => options?.id === older.id)).toHaveLength(0);
  expect(shows.mock.calls.filter(([, options]) => options?.id === followed.runId)).toHaveLength(1);
  view.unmount(); off();
});

// The restarted host is in the catalogue, so its targeted row stays in the merged history.
function catalogueHub(activities: Record<string, () => Promise<unknown[]> | unknown[]>) {
  const f = appHub(() => followed, () => followed);
  const source = new ActivitySource();
  const fetcher = async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/environments") return listJson([{ id: host.id, name: "Example", color: null, registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false }]);
    const listed = /^\/api\/environments\/([^/]+)\/activities$/.exec(url.pathname);
    if (listed) return listJson(await (activities[listed[1]!] ?? (() => []))());
    return (f.fetcher as unknown as typeof fetch)(input, init);
  };
  return { adapter: createHttpAdapter({ fetch: fetcher, eventSource: () => source as unknown as EventSource }), source };
}
const failedRun = (environmentId: string, id: string, at: string) => ({ id, environmentId, status: "failed", type: "project_redeploy", startedAt: at, createdAt: at, endedAt: at, resourceName: "Example app" });
const toastsFor = (shows: { mock: { calls: unknown[][] } }, id: string) => shows.mock.calls.filter(([, options]) => (options as { id?: string } | undefined)?.id === id);

// Row 1s: a hub snapshot frame read while the restarted host's history is still pending cannot
// seed that host's history.
it("keeps a pending host's older failures silent after a scoped snapshot", async () => {
  setAppLocale("en-US");
  let release!: () => void;
  const held = new Promise<void>((done) => { release = done; });
  let requested = false;
  const { adapter, source } = catalogueHub({
    "0": () => [failedRun("0", "run-old-hub", "2026-11-13T09:00:00Z")],
    [host.id]: async () => { requested = true; await held; return [failedRun(host.id, "run-old-remote", "2026-11-13T09:00:00Z")]; },
  });
  await adapter.getCurrentUser();
  const snapshots: CodedActionRun[][] = [];
  const off = adapter.subscribeActions((runs) => snapshots.push(runs));
  const shows = vi.spyOn(toast, "custom");
  const view = renderWithApp(<SignedInToasts />, adapter);
  await screen.findByTestId("toasts-ready");
  const aggregate = adapter.listActions();
  await waitFor(() => expect(requested).toBe(true));
  await act(async () => { await adapter.latestAgentRestart!(host.id); });
  await waitFor(() => expect(toastsFor(shows, followed.runId)).toHaveLength(1));
  act(() => source.send(undefined, "0", "snapshot"));
  await act(async () => { release(); await aggregate; });
  await waitFor(() => expect(snapshots.at(-1)?.some((run) => run.id === "run-old-remote")).toBe(true));
  expect(toastsFor(shows, "run-old-hub")).toHaveLength(0);
  expect(toastsFor(shows, "run-old-remote")).toHaveLength(0);
  expect(toastsFor(shows, followed.runId)).toHaveLength(1);
  view.unmount(); off();
});

// Row 1e: an empty first history completes the observation, so a later new failure still notifies.
it("notifies a new failure after an empty first history that retained the restart", async () => {
  setAppLocale("en-US");
  let release!: () => void;
  const held = new Promise<void>((done) => { release = done; });
  let requested = false;
  let hub: unknown[] = [];
  const { adapter } = catalogueHub({ "0": () => hub, [host.id]: async () => { requested = true; await held; return []; } });
  await adapter.getCurrentUser();
  const snapshots: CodedActionRun[][] = [];
  const off = adapter.subscribeActions((runs) => snapshots.push(runs));
  const shows = vi.spyOn(toast, "custom");
  const view = renderWithApp(<SignedInToasts />, adapter);
  await screen.findByTestId("toasts-ready");
  const aggregate = adapter.listActions();
  await waitFor(() => expect(requested).toBe(true));
  await act(async () => { await adapter.latestAgentRestart!(host.id); });
  await act(async () => { release(); await aggregate; });
  hub = [failedRun("0", "run-new-failure", "2026-11-13T11:00:00Z")];
  await act(async () => { await adapter.listActions(); });
  expect(snapshots.at(-1)?.some((run) => run.id === "run-new-failure")).toBe(true);
  await waitFor(() => expect(toastsFor(shows, "run-new-failure")).toHaveLength(1));
  expect(toastsFor(shows, followed.runId)).toHaveLength(1);
  view.unmount(); off();
});
