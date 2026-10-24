// Source: docklet-next/issues/102
// Revision: 5a51f9750ef391f272321daf1b1234a7e110c7f1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original real-consumer regressions informed by retained independent R3 probes.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor as testingWaitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { toast } from "sonner";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { useRunningActionCount } from "@/app/shell/use-nav-counts";
import { activity, activityFixture, deferred, detail, json } from "@/data/http/activity-reader.test-support";
import { useActionRuns } from "@/data/live";
import { queryKeys } from "@/data/query-keys";
import { AdapterContext } from "@/data/use-adapter";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { ActionToasts } from "./ActionToasts";
import { ActivityCenter } from "./ActivityCenter";
import { AppToaster } from "./AppToaster";
import { actionDialogStore } from "./action-dialog-store";
import { activityStore } from "./activity-store";
import { presentationStore } from "./run-presentation";

const waitFor = (assertion: () => void) => testingWaitFor(assertion, { timeout: 750 });
const appClient = () => new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30_000 }, mutations: { retry: false } } });
function fixture() {
  const f = activityFixture();
  f.state.lists.bad = []; f.state.lists.good = [];
  for (const status of ["failed", "running", "queued"] as const) {
    const id = `history-${status}`;
    f.state.lists.good.push({ ...activity("good", id), status });
    f.state.details[id] = { ...detail("good", id), activity: f.state.lists.good.at(-1)! };
  }
  const client = appClient();
  const discover = () => client.fetchQuery({ queryKey: queryKeys.currentUser, staleTime: 0, queryFn: () => f.adapter.getCurrentUser() });
  const mount = (children: React.ReactNode) => render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]} timeZone="UTC"><MemoryRouter>{children}</MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  return { ...f, client, discover, mount };
}
afterEach(() => act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); uiStyleStore.reset(); activityStore.reset(); vi.restoreAllMocks(); }));

const globalFailures = ["capability network", "capability500", "capability unavailable", "discovery network", "discovery500"] as const;
it.each(globalFailures.flatMap((failure) => (["toast", "dialog"] as const).map((presentation) => ({ failure, presentation }))))("$failure before first observation ($presentation) publishes health only, seeds recovery and still notifies new actions", async ({ failure, presentation }) => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: presentation });
  const shows = vi.spyOn(toast, "custom"); const observed = vi.fn(); const health = vi.fn();
  const off = f.adapter.subscribeActions(observed); const offHealth = f.adapter.subscribeActionReadHealth!(health);
  const view = f.mount(<><ActionToasts /><AppToaster /></>);
  try {
    // Repeat the complete first-observation sequence for a replacement identity too.
    for (const identity of ["operator", "next-operator"]) {
      if (identity !== "operator") { f.state.user.id = identity; await act(async () => { await f.discover(); }); }
      observed.mockClear();
      f.state.hold = async (path, response) => {
        const route = failure.startsWith("capability") ? "/api/capabilities" : "/api/environments";
        if (path !== route) return response;
        if (failure.endsWith("network")) throw new TypeError("Example unavailable source");
        return failure.endsWith("unavailable") ? json([]) : json(null, 500);
      };
      const failedPublications: number[] = [];
      for (let attempt = 0; attempt < 2; attempt++) {
        await act(async () => { await expect(f.adapter.listActions()).rejects.toBeTruthy(); });
        failedPublications.push(observed.mock.calls.length);
        expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: undefined, reason: "unavailable" }]);
      }
      f.state.hold = undefined;
      await act(async () => { f.sources.at(-1)!.environments(); await f.adapter.listActions(); });
      expect(shows).not.toHaveBeenCalled(); expect(actionDialogStore.get()).toEqual({ waiting: [] });
      expect(failedPublications).toEqual([0, 0]);
      expect(observed.mock.lastCall?.[0]).toHaveLength(3);
      expect(observed.mock.calls.every(([runs]) => runs.length === 3)).toBe(true);
      expect(health.mock.lastCall?.[0].failures).toEqual([]);
    }
    f.state.lists.good!.push(activity("good", "genuine-new")); f.state.details["genuine-new"] = detail("good", "genuine-new");
    await act(async () => { await f.adapter.listActions(); });
    if (presentation === "dialog") expect(actionDialogStore.get().runId).toBe("genuine-new");
    else { expect(shows).toHaveBeenCalledTimes(1); await waitFor(() => expect(document.querySelectorAll('[data-slot="action-toast"]').length).toBe(1)); }
    f.state.details["history-running"]!.activity.status = "failed";
    await act(async () => { await f.adapter.listActions(); });
    expect(shows).toHaveBeenCalledTimes(presentation === "dialog" ? 1 : 2);
  } finally { view.unmount(); off(); offHealth(); f.client.clear(); }
});

it("a failed refresh after a real snapshot retains rows; a successful empty first read is an observation", async () => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const shows = vi.spyOn(toast, "custom"); const observed = vi.fn(); const off = f.adapter.subscribeActions(observed);
  const view = f.mount(<><ActionToasts /><AppToaster /></>);
  try {
    await act(async () => { await f.adapter.listActions(); });
    const history = observed.mock.lastCall![0];
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); });
    f.state.hubStatus = 500;
    await act(async () => { await expect(f.adapter.listActions()).rejects.toBeTruthy(); });
    expect(observed.mock.lastCall?.[0]).toEqual(history); expect(shows).not.toHaveBeenCalled();
    f.state.hubStatus = 200; f.state.user.id = "empty-operator";
    await act(async () => { await f.discover(); });
    f.state.lists.good = [];
    await act(async () => { expect(await f.adapter.listActions()).toEqual([]); });
    expect(observed.mock.lastCall?.[0]).toEqual([]);
    f.state.lists.good = [activity("good", "after-empty")]; f.state.details["after-empty"] = detail("good", "after-empty");
    await act(async () => { await f.adapter.listActions(); });
    expect(shows).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(document.querySelectorAll('[data-slot="action-toast"]').length).toBe(1));
  } finally { view.unmount(); off(); f.client.clear(); }
});

function RunsAndCount() {
  const runs = useActionRuns(); const count = useRunningActionCount();
  return <><output aria-label="running actions">{count}</output><ul aria-label="observed actions">{runs.map((run) => <li key={run.id}>{run.id}</li>)}</ul></>;
}
const boundaries = ["logout same", "logout different", "auth401 same", "activity401 same", "replacement mounted"] as const;
it.each(boundaries)("%s cannot render old query-cache rows or header count before new history arrives", async (boundary) => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const content = <><RunsAndCount /><ActionToasts /><AppToaster /><ActivityCenter /></>;
  let view = f.mount(content);
  const shows = vi.spyOn(toast, "custom");
  const release = deferred<Response>(); const reached = deferred<Response>();
  try {
    await waitFor(() => expect(screen.getAllByRole("listitem").length).toBe(3));
    expect(screen.getByLabelText("running actions").textContent).toBe("2");
    const cached = f.client.getQueryData(queryKeys.actions); expect(cached).toHaveLength(3);
    if (boundary !== "replacement mounted") view.unmount();
    if (boundary.startsWith("logout")) { await act(async () => { await f.adapter.logout(); }); f.client.removeQueries({ queryKey: queryKeys.currentUser }); }
    if (boundary === "auth401 same") { f.state.authStatus = 401; await expect(f.discover()).rejects.toMatchObject({ outcome: { status: 401 } }); }
    if (boundary === "activity401 same") { f.state.failure = "page401"; await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); }
    f.state.authStatus = 200; f.state.failure = "";
    if (boundary === "logout different" || boundary === "replacement mounted") f.state.user.id = "next-operator";
    f.state.lists.good = [activity("good", "new-history")]; f.state.details["new-history"] = detail("good", "new-history");
    f.state.hold = async (path, response) => path.endsWith("/good/activities") ? (reached.resolve(response), release.promise.then(() => response)) : response;
    await act(async () => {
      if (boundary === "replacement mounted") await f.discover();
      else f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!"));
    });
    if (boundary !== "replacement mounted") view = f.mount(content);
    // Start the real new-generation query even if the old static-key query is still fresh.
    const pending = f.client.fetchQuery({ queryKey: queryKeys.actions, staleTime: 0, queryFn: () => f.adapter.listActions() });
    const captured = await reached.promise;
    expect(f.client.getQueryData(queryKeys.actions)).toEqual(cached);
    expect(screen.queryAllByRole("listitem").length).toBe(0);
    expect(screen.getByLabelText("running actions").textContent).toBe("0");
    expect(shows).not.toHaveBeenCalled();
    act(() => activityStore.set((s) => ({ ...s, open: true, tab: "actions" })));
    await screen.findByRole("dialog");
    expect(screen.queryByText(/Example good/) === null).toBe(true);
    f.state.hold = undefined;
    await act(async () => { release.resolve(captured); await pending; });
    await waitFor(() => expect(screen.getByLabelText("running actions").textContent).toBe("1"));
    expect(screen.queryByText("history-running") === null).toBe(true);
    expect(shows).not.toHaveBeenCalled();
  } finally { f.state.hold = undefined; release.resolve(new Response()); view.unmount(); f.client.clear(); }
});
