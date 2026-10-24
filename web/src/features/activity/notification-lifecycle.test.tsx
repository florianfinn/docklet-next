// Source: docklet-next/issues/102
// Revision: 22bb60b6c8cb7ed51a3eb0e465f07e696686b1e4
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original consumer regressions, informed by retained independent R2 probes.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor as testingWaitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { toast } from "sonner";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activity, activityFixture, detail, deferred } from "@/data/http/activity-reader.test-support";
import { queryKeys } from "@/data/query-keys";
import { AdapterContext } from "@/data/use-adapter";
import { ConnectedProfilePage } from "@/features/settings/ConnectedProfilePage";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { ActionToasts } from "./ActionToasts";
import { AppToaster } from "./AppToaster";
import { actionDialogStore } from "./action-dialog-store";
import { presentationStore } from "./run-presentation";

const waitFor = (assertion: () => void) => testingWaitFor(assertion, { timeout: 750 });
const count = () => document.querySelectorAll('[data-slot="action-toast"]').length;
function fixture(empty = false) {
  const shows = vi.spyOn(toast, "custom");
  const f = activityFixture();
  f.state.lists.bad = [];
  f.state.lists.good = [];
  if (!empty) for (const status of ["failed", "running", "queued"] as const) {
    const id = `history-${status}`;
    f.state.lists.good!.push({ ...activity("good", id), status });
    f.state.details[id] = { ...detail("good", id), activity: { ...activity("good", id), status } };
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000, refetchOnWindowFocus: false } } });
  const discover = () => client.fetchQuery({ queryKey: queryKeys.currentUser, staleTime: 0, queryFn: () => f.adapter.getCurrentUser() });
  const mount = (profile = false) => render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]} timeZone="UTC"><MemoryRouter><ActionToasts /><AppToaster />{profile && <ConnectedProfilePage />}</MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  const load = () => act(async () => { await f.adapter.listActions(); });
  return { ...f, client, discover, mount, load, shows };
}
afterEach(() => act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); uiStyleStore.reset(); vi.restoreAllMocks(); }));

it.each(["discovery", "login", "loaded before shell"])("%s seeds real history without historical toasts or major dialog offers", async (start) => {
  const f = fixture(); setUiStyle({ actionPresentationOverride: "dialog" });
  if (start === "login") f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!"));
  else await f.discover();
  if (start === "loaded before shell") await f.adapter.listActions();
  const observed = vi.fn(); const off = f.adapter.subscribeActions(observed);
  const view = f.mount();
  try {
    if (start !== "loaded before shell") expect(observed).not.toHaveBeenCalled();
    await f.load();
    await waitFor(() => expect(observed.mock.lastCall?.[0]).toHaveLength(3));
    expect(count()).toBe(0); expect(f.shows).not.toHaveBeenCalled(); expect(actionDialogStore.get()).toEqual({ waiting: [] });
    expect(observed.mock.calls.every(([runs]) => runs.length === 3)).toBe(true);
  } finally { view.unmount(); off(); f.client.clear(); }
});

it.each([false, true])("real initial history (empty=%s) still allows genuine new major actions and terminal notifications", async (empty) => {
  const f = fixture(empty); await f.discover(); setUiStyle({ actionPresentationOverride: "dialog" });
  const view = f.mount();
  try {
    await f.load(); expect(count()).toBe(0);
    f.state.lists.good!.push(activity("good", "new-major")); f.state.details["new-major"] = detail("good", "new-major");
    await f.load();
    await waitFor(() => expect(actionDialogStore.get().runId).toBe("new-major"));
    f.state.lists.good!.push(activity("good", "next-major")); f.state.details["next-major"] = detail("good", "next-major");
    await f.load();
    expect(actionDialogStore.get()).toEqual({ runId: "new-major", waiting: ["next-major"] });
    await waitFor(() => expect(count()).toBe(1));
    await userEvent.click(within(document.querySelector<HTMLElement>('[data-slot="action-toast"]')!).getByRole("button", { name: "Close notice" }));
    await waitFor(() => expect(count()).toBe(0));
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); });
    await f.load(); expect(f.shows).toHaveBeenCalledTimes(1); expect(count()).toBe(0);
    f.state.details["next-major"]!.activity.status = "failed";
    await f.load(); await waitFor(() => expect(count()).toBe(1));
    expect(document.querySelector('[data-slot="action-toast"]')?.getAttribute("data-status")).toBe("failed");
    expect(actionDialogStore.get()).toEqual({ runId: "new-major", waiting: ["next-major"] });
  } finally { view.unmount(); f.client.clear(); }
});

it("a real password gesture preserves history, dismissed notices and nonempty observations in the mounted shell", async () => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const observed = vi.fn(); const off = f.adapter.subscribeActions(observed); const changes = vi.fn(); const offChanges = f.adapter.subscribeChanges(changes);
  const view = f.mount(true);
  try {
    await screen.findByRole("heading", { name: "Profile" }); await f.load(); expect(count()).toBe(0);
    f.state.lists.good!.push(activity("good", "new-run")); f.state.details["new-run"] = detail("good", "new-run");
    await f.load(); await waitFor(() => expect(count()).toBe(1));
    await userEvent.click(within(document.querySelector<HTMLElement>('[data-slot="action-toast"]')!).getByRole("button", { name: "Close notice" }));
    await waitFor(() => expect(count()).toBe(0));
    const publications = observed.mock.calls.length; const old = f.sources[0]!;
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Current password"), "Example-password1!");
    await user.type(screen.getByLabelText("New password"), "New-example-password2!");
    await user.type(screen.getByLabelText("Repeat password"), "New-example-password2!");
    await user.click(screen.getByRole("button", { name: "Change password" }));
    await screen.findByRole("status");
    expect(screen.queryByRole("heading", { name: "Profile" }) !== null).toBe(true);
    expect(observed.mock.calls.slice(publications)).toEqual([]); expect(count()).toBe(0);
    expect(f.sources).toHaveLength(2); expect(old.close).toHaveBeenCalledTimes(1);
    const before = f.requests.length; old.send(); old.environments(); expect(f.requests).toHaveLength(before);
    await act(async () => { f.sources[1]!.open(); });
    await f.load(); expect(f.shows).toHaveBeenCalledTimes(1); expect(count()).toBe(0);
    expect(observed.mock.calls.slice(publications).every(([runs]) => runs.length === 4)).toBe(true);
    expect(changes).toHaveBeenCalled();
    await act(async () => { await f.discover(); f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!")); });
    await f.load(); expect(f.shows).toHaveBeenCalledTimes(1); expect(count()).toBe(0);
    expect(observed.mock.calls.slice(publications).every(([runs]) => runs.length === 4)).toBe(true);
  } finally { view.unmount(); off(); offChanges(); f.client.clear(); }
});

it.each(["replacement", "logout", "auth401", "activity401"])("%s clears old identity and seeds the next authenticated shell's history", async (boundary) => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const observed = vi.fn(); const off = f.adapter.subscribeActions(observed); let view = f.mount();
  try {
    await f.load();
    f.state.lists.good!.push(activity("good", "old-notice")); f.state.details["old-notice"] = detail("good", "old-notice");
    await f.load(); await waitFor(() => expect(count()).toBe(1));
    const held = deferred<Response>(); const reached = deferred<Response>();
    f.state.hold = async (path, response) => path.endsWith("/good/activities/history-failed") ? (reached.resolve(response), held.promise) : response;
    const pending = f.adapter.listActions();
    const rejected = expect(pending).rejects.toMatchObject(boundary === "activity401" ? { outcome: { status: 401 } } : { name: "Error" });
    const captured = await reached.promise; f.state.hold = undefined;
    if (boundary === "logout") await act(async () => { await f.adapter.logout(); });
    if (boundary === "auth401") { f.state.authStatus = 401; await act(async () => { await expect(f.discover()).rejects.toMatchObject({ outcome: { status: 401 } }); }); }
    if (boundary === "activity401") {
      f.state.failure = "page401";
      await act(async () => {
        // Signal a trailing read before releasing the shared pre-notification snapshot.
        f.sources[0]!.open();
        held.resolve(captured);
        await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } });
      });
      await rejected;
      expect(observed.mock.lastCall?.[0]).toEqual([]);
    }
    f.state.lists.good = [{ ...activity("good", "new-history"), status: "failed" }]; f.state.details["new-history"] = { ...detail("good", "new-history"), activity: { ...activity("good", "new-history"), status: "failed" } };
    f.state.user.id = "next-operator"; f.state.authStatus = 200; f.state.failure = "";
    if (boundary === "replacement") await act(async () => { await f.discover(); });
    else {
      view.unmount();
      f.client.clear(); f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!"));
      view = f.mount();
    }
    expect(observed.mock.lastCall?.[0]).toEqual([]);
    held.resolve(captured); await rejected;
    await f.load();
    expect(f.shows).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(count()).toBe(0));
    expect(observed.mock.lastCall?.[0].map((r: { id: string }) => r.id)).toEqual(["new-history"]);
    expect(actionDialogStore.get()).toEqual({ waiting: [] });
  } finally { view.unmount(); off(); f.client.clear(); }
});


it.each(["surface", "dialog"])("%s changes before the first observation cannot initialize toast history", async (source) => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const view = f.mount();
  try {
    act(() => {
      if (source === "surface") presentationStore.set((s) => ({ ...s, surfaces: { "good~example": 1 } }));
      else actionDialogStore.set({ runId: "unrelated-example", waiting: [] });
    });
    await f.load();
    expect(f.shows).not.toHaveBeenCalled(); expect(count()).toBe(0);
  } finally { view.unmount(); f.client.clear(); }
});

it("activity401 then same-user login into a fresh shell seeds real history", async () => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  let view = f.mount();
  try {
    await f.load(); expect(f.shows).not.toHaveBeenCalled();
    f.state.failure = "page401";
    await act(async () => { await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); });
    view.unmount(); f.state.failure = "";
    f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!"));
    const observed = vi.fn(); const off = f.adapter.subscribeActions(observed);
    try {
      view = f.mount(); await f.load();
      expect(observed.mock.lastCall?.[0]).toHaveLength(3);
      expect(f.shows).not.toHaveBeenCalled(); expect(count()).toBe(0);
    } finally { off(); }
    f.state.lists.good!.push(activity("good", "after-relogin")); f.state.details["after-relogin"] = detail("good", "after-relogin");
    await f.load(); await waitFor(() => expect(count()).toBe(1));
    expect(f.shows).toHaveBeenCalledTimes(1);
  } finally { view.unmount(); f.client.clear(); }
});

it("identity replacement still notifies genuinely new runs in the mounted shell", async () => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const view = f.mount();
  try {
    await f.load(); f.state.user.id = "next-operator";
    await act(async () => { await f.discover(); });
    await f.load(); expect(f.shows).not.toHaveBeenCalled();
    f.state.lists.good!.push(activity("good", "genuine-new")); f.state.details["genuine-new"] = detail("good", "genuine-new");
    await f.load();
    expect(f.shows).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(count()).toBe(1));
  } finally { view.unmount(); f.client.clear(); }
});

it("identity replacement dismisses the prior owned toast even when the same UUID survives", async () => {
  const f = fixture(); await f.discover(); setUiStyle({ actionPresentationOverride: "toast" });
  const dismiss = vi.spyOn(toast, "dismiss");
  const view = f.mount();
  try {
    await f.load();
    f.state.lists.good!.push(activity("good", "same-run")); f.state.details["same-run"] = detail("good", "same-run");
    await f.load(); await waitFor(() => expect(count()).toBe(1));
    f.state.user.id = "next-operator";
    await act(async () => { await f.discover(); });
    await f.load();
    expect(dismiss).toHaveBeenCalledWith("same-run");
    await waitFor(() => expect(count()).toBe(0));
    expect(f.shows).toHaveBeenCalledTimes(1);
    f.state.details["same-run"]!.activity.status = "failed";
    await f.load();
    expect(f.shows).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(count()).toBe(1));
    expect(document.querySelector('[data-slot="action-toast"]')?.getAttribute("data-status")).toBe("failed");
  } finally { view.unmount(); f.client.clear(); }
});
