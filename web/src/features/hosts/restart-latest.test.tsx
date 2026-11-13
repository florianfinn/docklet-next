// Source: docklet-next/issues/429
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original reload tests of the host restart panel through the HTTP adapter.

import { screen, waitFor } from "@testing-library/react";
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
// A signed-in hub for the real toast consumer: `latest` answers the latest read, `detail` the follow reads.
function appHub(latest: () => Restart, detail: () => Restart) {
  const rest = stubHub(() => answer(latest()));
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data, pagination: { totalItems: Array.isArray(data) ? data.length : 0 } }), { headers: { "Content-Type": "application/json" } });
    if (url.pathname === "/api/auth/me") return json({ id: "example-operator", username: "operator", requiresPasswordChange: false, locale: "en-US", isAdmin: true });
    if (url.pathname === "/api/settings/timezone") return json({ timezone: "UTC" });
    if (url.pathname === "/api/capabilities") return json(["listActions", "listHosts", "restartAgent", "reconnectHost"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (url.pathname.endsWith("/activities")) return json([]);
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
