// Source: docklet-next/issues/102
// Revision: 5a51f9750ef391f272321daf1b1234a7e110c7f1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original real app-query and route-consumer renewal regressions informed by independent R3 probes.

import { CancelledError, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, render, screen, waitFor as testingWaitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { MemoryRouter, Route, Routes } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it } from "vitest";
import { AuthGate } from "@/app/AuthGate";
import { SetupGate } from "@/app/SetupGate";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "@/data/AdapterProvider";
import { activityFixture, deferred } from "@/data/http/activity-reader.test-support";
import { queryKeys } from "@/data/query-keys";
import { ConnectedProfilePage } from "@/features/settings/ConnectedProfilePage";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";

const waitFor = (assertion: () => void) => testingWaitFor(assertion, { timeout: 750 });
function ClientCapture({ capture }: { capture: (client: QueryClient) => void }) {
  const client = useQueryClient(); useEffect(() => capture(client), [client, capture]); return null;
}
function fixture() {
  const f = activityFixture(); const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input: RequestInfo | URL) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/0/settings"
    ? new Response(JSON.stringify([{ key: "setupCompleted", value: "true" }]), { headers: { "content-type": "application/json" } })
    : original(input));
  let client!: QueryClient;
  const capture = (value: QueryClient) => { client = value; };
  const mount = (children: React.ReactNode) => render(<IntlProvider locale="en-US" messages={catalogs["en-US"]} timeZone="UTC"><MemoryRouter><AdapterProvider adapter={f.adapter}><ClientCapture capture={capture} />{children}</AdapterProvider></MemoryRouter></IntlProvider>);
  return { ...f, mount, client: () => client };
}
afterEach(() => act(() => activityStore.reset()));

it.each((["password", "login"] as const).flatMap((renewal) => (["shell", "setup"] as const).map((consumer) => ({ renewal, consumer }))))("known $renewal renewal revalidates AuthGate/$consumer and keeps the actual app ChangeBridge active", async ({ renewal, consumer }) => {
  const f = fixture();
  if (consumer === "setup") f.state.user.requiresPasswordChange = true;
  const view = f.mount(<Routes>
    <Route element={<AuthGate />}><Route index element={<><p>Shell content</p><ConnectedProfilePage /></>} /></Route>
    <Route path="/setup" element={<SetupGate />} /><Route path="/login" element={<p>Login page</p>} />
  </Routes>);
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    if (consumer === "shell") await screen.findByRole("heading", { name: "Profile" });
    else await screen.findByRole("heading", { name: "New administrator password" });
    const client = f.client();
    f.state.hold = async (path, response) => path.endsWith("/auth/me") ? (reached.resolve(response), held.promise) : response;
    const before = f.requests.filter((path) => path === "/api/auth/me").length;
    const pending = client.invalidateQueries({ queryKey: queryKeys.currentUser });
    const captured = await reached.promise; f.state.hold = undefined;
    const old = f.sources.at(-1)!;
    if (renewal === "password" && consumer === "shell") {
      const user = userEvent.setup();
      await user.type(screen.getByLabelText("Current password"), "Example-password1!");
      await user.type(screen.getByLabelText("New password"), "New-example-password2!");
      await user.type(screen.getByLabelText("Repeat password"), "New-example-password2!");
      await user.click(screen.getByRole("button", { name: "Change password" }));
      await screen.findByRole("status");
    } else await act(async () => {
      if (renewal === "password") await f.adapter.changePassword("Example-password1!", "New-example-password2!");
      else await f.adapter.login("operator", "Example-password1!");
    });
    f.state.user.username = "renewed-operator";
    await act(async () => { held.resolve(captured); await pending.catch(() => undefined); });
    expect(client.getQueryState(queryKeys.currentUser)).toMatchObject({ status: "success", fetchStatus: "idle", error: null, data: { username: "renewed-operator" } });
    expect(f.requests.filter((path) => path === "/api/auth/me").length - before).toBe(2);
    expect(screen.queryByText("The session could not be checked.") === null).toBe(true);
    expect(screen.queryByText("First-run setup status could not be checked.") === null).toBe(true);
    if (consumer === "shell") expect(screen.queryByRole("heading", { name: "Profile" }) !== null).toBe(true);
    else expect(screen.queryByRole("heading", { name: "New administrator password" }) !== null).toBe(true);
    expect(old.close).toHaveBeenCalledTimes(1);
    client.setQueryData(queryKeys.events, []);
    await act(async () => { f.sources.at(-1)!.send(); });
    expect(client.getQueryState(queryKeys.events)?.isInvalidated).toBe(true);
  } finally { f.state.hold = undefined; held.resolve(new Response()); view.unmount(); f.client().clear(); }
});

it.each([false, true].flatMap((prior) => (["password", "login"] as const).map((renewal) => ({ prior, renewal }))))("empty ActivityCenter held across $renewal renewal (prior successful empty=$prior) settles success without a false failure", async ({ prior, renewal }) => {
  const f = fixture(); f.state.lists.good = []; f.state.lists.bad = [];
  await f.adapter.getCurrentUser();
  const held = deferred<Response>(); const reached = deferred<Response>();
  if (!prior) f.state.hold = async (path, response) => path.endsWith("/good/activities") ? (reached.resolve(response), held.promise) : response;
  act(() => activityStore.set((s) => ({ ...s, open: true, tab: "actions" })));
  const view = f.mount(<ActivityCenter />);
  try {
    const client = f.client(); let pending: Promise<unknown>;
    if (prior) {
      await screen.findByText("No actions in this session yet.");
      f.state.hold = async (path, response) => path.endsWith("/good/activities") ? (reached.resolve(response), held.promise) : response;
      pending = client.invalidateQueries({ queryKey: queryKeys.actions });
    } else pending = client.getQueryCache().find({ queryKey: queryKeys.actions })!.promise!;
    const captured = await reached.promise; f.state.hold = undefined;
    const before = f.requests.filter((path) => path.startsWith("/api/environments/good/activities?")).length;
    await act(async () => {
      if (renewal === "password") await f.adapter.changePassword("Example-password1!", "New-example-password2!");
      else await f.adapter.login("operator", "Example-password1!");
    });
    await act(async () => { held.resolve(captured); await pending.catch(() => undefined); });
    expect(client.getQueryState(queryKeys.actions)).toMatchObject({ status: "success", fetchStatus: "idle", error: null, data: [] });
    expect(f.requests.filter((path) => path.startsWith("/api/environments/good/activities?")).length - before).toBe(1);
    await waitFor(() => expect(screen.queryByText("No actions in this session yet.") !== null).toBe(true));
    expect(screen.queryByText("Data could not be loaded") === null).toBe(true);
  } finally { f.state.hold = undefined; held.resolve(new Response()); view.unmount(); f.client().clear(); }
});

it.each([500, 401])("a genuine %s on renewal revalidation remains visible and settled", async (status) => {
  const f = fixture();
  const view = f.mount(<Routes><Route element={<AuthGate />}><Route index element={<p>Shell content</p>} /></Route><Route path="/login" element={<p>Login page</p>} /></Routes>);
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    await screen.findByText("Shell content");
    const client = f.client();
    f.state.hold = async (path, response) => path.endsWith("/auth/me") ? (reached.resolve(response), held.promise) : response;
    const pending = client.invalidateQueries({ queryKey: queryKeys.currentUser });
    const captured = await reached.promise; f.state.hold = undefined;
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); });
    f.state.authStatus = status;
    await act(async () => { held.resolve(captured); await pending.catch(() => undefined); });
    expect(client.getQueryState(queryKeys.currentUser)).toMatchObject({ status: "error", fetchStatus: "idle" });
    if (status === 401) await screen.findByText("Login page");
    else await screen.findByText("The session could not be checked.");
    expect(screen.queryByText("Shell content") === null).toBe(true);
  } finally { f.state.hold = undefined; held.resolve(new Response()); view.unmount(); f.client().clear(); }
});

it.each(["logout", "activity401", "replacement"] as const)("a held auth read across %s then renewal still cancels without re-reading across the identity boundary", async (boundary) => {
  const f = fixture(); await f.adapter.getCurrentUser();
  const view = f.mount(<p>Query owner</p>);
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    await waitFor(() => expect(f.client().getQueryState(queryKeys.currentUser)?.status).toBe("success"));
    const client = f.client();
    f.state.hold = async (path, response) => path.endsWith("/auth/me") ? (reached.resolve(response), held.promise) : response;
    const pending = client.fetchQuery({ queryKey: queryKeys.currentUser, staleTime: 0, queryFn: () => f.adapter.getCurrentUser() });
    const rejection = expect(pending).rejects.toBeInstanceOf(CancelledError);
    const captured = await reached.promise; f.state.hold = undefined;
    if (boundary === "logout") await f.adapter.logout();
    if (boundary === "activity401") { f.state.failure = "page401"; await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); f.state.failure = ""; }
    if (boundary === "replacement") { f.state.user.id = "next-operator"; await f.adapter.getCurrentUser(); }
    await f.adapter.login("operator", "Example-password1!");
    const requests = f.requests.length;
    await act(async () => { held.resolve(captured); await rejection; });
    expect(f.requests).toHaveLength(requests);
    expect(client.getQueryState(queryKeys.currentUser)).toMatchObject({ status: "error", fetchStatus: "idle" });
  } finally { f.state.hold = undefined; held.resolve(new Response()); view.unmount(); f.client().clear(); }
});
