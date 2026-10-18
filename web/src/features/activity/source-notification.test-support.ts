// Source: docklet-next/issues/102
// Revision: 1819a8a7ed4f4b8116c2195c9ba5e3b16a03120a
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original real notification consumer fixtures for source-local initial history.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, waitFor as testingWaitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement, Fragment } from "react";
import { MemoryRouter } from "react-router";
import { toast } from "sonner";
import { IntlProvider } from "use-intl";
import { expect, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activity, activityFixture, detail, json, type Activity } from "@/data/http/activity-reader.test-support";
import { queryKeys } from "@/data/query-keys";
import { AdapterContext } from "@/data/use-adapter";
import { setUiStyle } from "@/shared/preferences/ui-style-store";
import { ActionToasts } from "./ActionToasts";
import { ActivityCenter } from "./ActivityCenter";
import { AppToaster } from "./AppToaster";
import { activityStore } from "./activity-store";

export const waitFor = (assertion: () => void) => testingWaitFor(assertion, { timeout: 750 });
export const toastCount = () => document.querySelectorAll('[data-slot="action-toast"]').length;
export type Fault = "network" | "page502" | "page500" | "page403" | "page401" | "detail404" | "mapping" | "partial" | "marked-first" | "marked-later";
export function sourceFixture(reverse = false) {
  const f = activityFixture(false, reverse);
  f.state.hosts = reverse ? ["edge", "bad", "good"] : ["good", "bad", "edge"];
  f.state.lists = { "0": [], good: [], bad: [], edge: [] };
  const faults = new Map<string, Fault>();
  f.state.hold = async (path, response, url) => {
    if (path === "/api/environments" && response.ok) {
      const body = await response.json();
      return json(body.data.map((host: { id: string }) => ({ ...host, isEdge: host.id === "edge" })));
    }
    const match = path.match(/\/environments\/([^/]+)\/activities(?:\/(.+))?$/);
    if (!match) return response;
    const [, host, id] = match; const fault = faults.get(host!);
    if (!id && fault === "network") throw new TypeError("Example source unavailable");
    if (!id && fault?.startsWith("page")) return json(null, Number(fault.slice(4)));
    if (id && fault === "detail404") return json(null, 404);
    if (id && fault === "mapping") return json({ ...f.state.details[id], outputTruncated: undefined });
    if (!id && (fault === "partial" || fault?.startsWith("marked-"))) {
      const items = f.state.lists[host!] ?? [];
      const later = url.searchParams.get("start") !== "0";
      const paged = fault !== "partial";
      const page = paged ? items.slice(later ? 1 : 0, later ? 2 : 1) : items.slice(0, 1);
      const result = json(page, 200, paged ? Math.min(items.length, 2) : page.length);
      if (fault === "partial" || later === (fault === "marked-later")) result.headers.set("X-Docklet-Host-Reason", "agent.unavailable");
      return result;
    }
    return response;
  };
  const shows = vi.spyOn(toast, "custom");
  const observed = vi.fn(); const health = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30_000 }, mutations: { retry: false } } });
  const discover = () => client.fetchQuery({ queryKey: queryKeys.currentUser, staleTime: 0, queryFn: () => f.adapter.getCurrentUser() });
  const put = (host: string, id: string, status: Activity["status"] = "running") => {
    const row = { ...activity(host, id), status };
    f.state.lists[host]!.push(row); f.state.details[id] = { ...detail(host, id), activity: row };
  };
  const history = (host: string) => { for (const status of ["failed", "running", "queued"] as const) put(host, `${host}-history-${status}`, status); };
  const ids = () => shows.mock.calls.map(([, options]) => options?.id);
  const load = () => act(async () => { await f.adapter.listActions(); });
  const mount = (center = false, presentation: "toast" | "dialog" = "toast") => {
    setUiStyle({ actionPresentationOverride: presentation });
    const off = f.adapter.subscribeActions(observed); const offHealth = f.adapter.subscribeActionReadHealth?.(health);
    const content = createElement(Fragment, null, createElement(ActionToasts), createElement(AppToaster), center && createElement(ActivityCenter));
    const router = createElement(MemoryRouter, null, content);
    const intl = createElement(IntlProvider, { locale: "en-US", messages: catalogs["en-US"], timeZone: "UTC", children: router });
    const view = render(createElement(AdapterContext.Provider, { value: f.adapter }, createElement(QueryClientProvider, { client }, intl)));
    return { ...view, stop: () => { view.unmount(); off(); offHealth?.(); } };
  };
  const openCenter = () => act(() => activityStore.set((s) => ({ ...s, open: true, tab: "actions" })));
  const dismiss = async (id: string) => {
    const node = document.querySelector<HTMLElement>(`[data-sonner-toast][data-visible="true"] [data-slot="action-toast"][data-status]`);
    expect(ids()).toContain(id); expect(node !== null).toBe(true);
    await userEvent.click(within(node!).getByRole("button", { name: "Close notice" }));
    await waitFor(() => expect(toastCount()).toBe(0));
  };
  return { ...f, faults, shows, observed, health, client, discover, put, history, ids, load, mount, openCenter, dismiss };
}
