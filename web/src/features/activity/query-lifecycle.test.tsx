// Source: docklet-next/issues/102
// Revision: 22bb60b6c8cb7ed51a3eb0e465f07e696686b1e4
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original settled-query and Retry regressions, informed by retained independent R2 probe.

import { CancelledError, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor as testingWaitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { activityFixture, deferred } from "@/data/http/activity-reader.test-support";
import { AdapterContext } from "@/data/use-adapter";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";

const waitFor = (assertion: () => void) => testingWaitFor(assertion, { timeout: 750 });
const clientFor = (success = vi.fn()) => new QueryClient({ queryCache: new QueryCache({ onSuccess: success }), defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30_000 } } });
afterEach(() => act(() => activityStore.reset()));

it.each([false, true])("renewed ActivityCenter read (prior data=%s) revalidates, settles idle and actual Retry works twice", async (prior) => {
  const f = activityFixture(true); f.state.failure = "page502";
  await f.adapter.getCurrentUser();
  const client = clientFor();
  const held = deferred<Response>(); const reached = deferred<Response>();
  const hold = () => { f.state.hold = async (path, response) => path.endsWith("/good/activities/run-good") ? (reached.resolve(response), held.promise) : response; };
  if (!prior) hold();
  act(() => activityStore.set((s) => ({ ...s, open: true, tab: "actions" })));
  const errors = vi.fn(); window.addEventListener("docklet:adapter-error", errors);
  const view = render(<AdapterContext.Provider value={f.adapter}><QueryClientProvider client={client}><IntlProvider locale="en-US" messages={catalogs["en-US"]} timeZone="UTC"><MemoryRouter><ActivityCenter /></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
  const retry = () => within(screen.getByRole("alert")).getByRole<HTMLButtonElement>("button", { name: "Retry activity reads" });
  try {
    if (prior) {
      await screen.findByRole("alert"); await waitFor(() => expect(retry().disabled).toBe(false));
      hold(); await userEvent.click(retry());
    }
    const captured = await reached.promise;
    const query = client.getQueryCache().find({ queryKey: ["actions"] })!;
    const pending = query.promise!;
    expect(query.state.fetchStatus).toBe("fetching");
    f.state.hold = undefined;
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); f.sources.at(-1)!.open(); });
    await act(async () => { held.resolve(captured); await pending.catch(() => undefined); });
    expect(query.state.fetchStatus).toBe("idle");
    expect(query.state.status).toBe("success");
    await screen.findByRole("alert");
    expect(client.getQueryData(["actions"])).toEqual(await f.adapter.listActions());
    await waitFor(() => expect(retry().disabled).toBe(false));
    expect(screen.queryByText(/Example good/) !== null).toBe(true);
    const requests = f.requests.length;
    await userEvent.click(retry());
    await waitFor(() => expect(query.state.status).toBe("success"));
    await waitFor(() => expect(retry().disabled).toBe(false));
    expect(f.requests.length).toBeGreaterThan(requests);
    const next = f.requests.length;
    await userEvent.click(retry());
    await waitFor(() => expect(f.requests.length).toBeGreaterThan(next));
    await waitFor(() => expect(query.state.fetchStatus).toBe("idle"));
    f.state.failure = "";
    await userEvent.click(retry());
    await waitFor(() => expect(screen.queryByRole("alert") === null).toBe(true));
    expect(query.state.status).toBe("success"); expect(query.state.fetchStatus).toBe("idle");
    expect(errors).not.toHaveBeenCalled();
  } finally { view.unmount(); client.clear(); window.removeEventListener("docklet:adapter-error", errors); }
});

it.each(["logout", "identity", "activity401"])("held current-user query across %s rejects and settles idle without stale success", async (boundary) => {
  const f = activityFixture(); await f.adapter.getCurrentUser();
  const success = vi.fn(); const client = clientFor(success); const stop = f.adapter.subscribeActions(vi.fn());
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    f.state.hold = async (path, response) => path.endsWith("/auth/me") ? (reached.resolve(response), held.promise) : response;
    const pending = client.fetchQuery({ queryKey: ["current-user"], queryFn: () => f.adapter.getCurrentUser() });
    const rejected = expect(pending).rejects.toBeInstanceOf(CancelledError); const response = await reached.promise;
    f.state.hold = undefined;
    if (boundary === "logout") await f.adapter.logout();
    if (boundary === "identity") { f.state.user.id = "another-operator"; await f.adapter.getCurrentUser(); }
    if (boundary === "activity401") { f.state.failure = "page401"; await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); }
    held.resolve(response); await rejected;
    expect(client.getQueryState(["current-user"])).toMatchObject({ status: "error", fetchStatus: "idle", data: undefined });
    expect(success).not.toHaveBeenCalled();
    f.state.failure = "";
    expect((await client.fetchQuery({ queryKey: ["current-user"], queryFn: () => f.adapter.getCurrentUser() })).id).toBe(f.state.user.id);
    expect(client.getQueryState(["current-user"])?.fetchStatus).toBe("idle");
  } finally { stop(); client.clear(); }
});

it.each(["logout", "unsubscribe", "activity401", "replacement"] as const)("held actions query across %s and subsequent renewal rejects old work without a new read", async (boundary) => {
  const f = activityFixture(); await f.adapter.getCurrentUser();
  let stop = f.adapter.subscribeActions(vi.fn()); const client = clientFor();
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    f.state.hold = async (path, response) => path.endsWith("/good/activities/run-good") ? (reached.resolve(response), held.promise) : response;
    const pending = client.fetchQuery({ queryKey: ["actions"], queryFn: () => f.adapter.listActions() });
    const rejected = boundary === "activity401" ? expect(pending).rejects.toMatchObject({ outcome: { status: 401 } }) : expect(pending).rejects.toBeInstanceOf(CancelledError);
    const response = await reached.promise; f.state.hold = undefined;
    if (boundary === "logout") await f.adapter.logout();
    if (boundary === "unsubscribe") { stop(); stop = () => {}; }
    if (boundary === "activity401") {
      vi.useFakeTimers();
      f.state.failure = "page401";
      f.sources[0]!.open();
      const failure = f.adapter.listActions();
      await expect(failure).rejects.toMatchObject({ outcome: { status: 401 } });
      await rejected;
      f.state.failure = "";
    }
    if (boundary === "replacement") { f.state.user.id = "next-operator"; await f.adapter.getCurrentUser(); }
    await f.adapter.login("operator", "Example-password1!");
    const requests = f.requests.length;
    held.resolve(response); await rejected;
    if (boundary === "activity401") await vi.advanceTimersByTimeAsync(0);
    expect(f.requests).toHaveLength(requests);
    expect(client.getQueryState(["actions"])).toMatchObject({ status: "error", fetchStatus: "idle", data: undefined });
  } finally { if (boundary === "activity401") vi.useRealTimers(); f.state.hold = undefined; held.resolve(new Response()); stop(); client.clear(); }
});
