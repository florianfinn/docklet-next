// Source: docklet-next/docs/design/identity.md
// Revision: 274674bb319eb7b1b854688ebc945e831c4dd53f
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original provider/route regressions for successful-null and confirmed identity transitions.

import { useState } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AuthGate } from "@/app/AuthGate";
import { setAppLocale } from "@/app/i18n/locale-store";
import { catalogs } from "@/app/i18n/catalog";
import { LogoutDialog } from "@/app/shell/LogoutDialog";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { renderRoutes } from "@/test/render";
import { deferred } from "./http/activity-reader.test-support";
import { envelope, response } from "./http/runtime-commands.test-support";
import { sessionFixture } from "./http/session-lifecycle.test-support";
import { identityLifetime } from "./identity-lifetime";
import { queryKeys } from "./query-keys";

const configured = () => response([{ key: "setupCompleted", value: "true" }]);
afterEach(() => setAppLocale("de"));

it.each((["en-US", "de"] as const).flatMap((locale) => [true, false].map((cold) => ({ locale, cold }))))("proxy /login explains rejection in $locale (cold=$cold)", async ({ locale, cold }) => {
  setAppLocale(locale);
  const f = sessionFixture((url) => url.pathname.endsWith("/auth/mode") ? envelope({ mode: "proxy" }) : undefined);
  f.state.authenticated = !cold;
  let client!: QueryClient;
  function Login() { client = useQueryClient(); return <ProductLoginPage />; }
  const view = renderRoutes([{ path: "/login", element: <Login /> }], ["/login"], f.adapter);
  if (!cold) {
    await waitFor(() => expect(client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: f.user.id }));
    f.state.authenticated = false;
    await act(async () => { await client.refetchQueries({ queryKey: queryKeys.currentUser }); });
  }
  await waitFor(() => expect(client.getQueryState(queryKeys.currentUser)).toMatchObject({ status: "success", data: null }));
  // Assert the settled result directly: the regression leaves a perpetual loading alert.
  expect(screen.getByRole("alert").textContent).toBe(catalogs[locale].login.proxyRejected);
  expect(document.body.textContent?.includes(catalogs[locale].login.loading)).toBe(false);
  view.unmount();
});

it("a held currentUser replacement preserves /stacks and the confirmed query while other identity data resets", async () => {
  const held = deferred<Response>(); const started = deferred<void>();
  let replacing = false;
  const f = sessionFixture((url) => {
    if (url.pathname.endsWith("/environments/0/settings")) return configured();
    if (replacing && url.pathname.endsWith("/auth/me")) { started.resolve(); return held.promise; }
  });
  let client!: QueryClient;
  const loginRender = vi.fn();
  function Page() { client = useQueryClient(); return <p>Stacks page</p>; }
  function Login() { loginRender(); return <ProductLoginPage />; }
  const view = renderRoutes([
    { path: "/login", element: <Login /> },
    { path: "/", element: <AuthGate />, children: [{ path: "stacks", element: <Page /> }, { path: "dashboard", element: <p>Dashboard</p> }] },
  ], ["/stacks"], f.adapter);
  await screen.findByText("Stacks page");
  client.setQueryData(queryKeys.events, ["old private event"]);
  const identities: unknown[] = [];
  const offCache = client.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] === queryKeys.currentUser[0]) identities.push(event.query.state.data);
  });
  const synchronousReset = vi.fn(() => expect(client.getQueryData(queryKeys.events)).toBeUndefined());
  const offIdentity = identityLifetime(f.adapter).listen(synchronousReset);
  replacing = true;
  const refetch = client.refetchQueries({ queryKey: queryKeys.currentUser });
  await started.promise;
  expect(view.router.state.location.pathname).toBe("/stacks");
  const identityReads = f.calls.filter(({ path }) => path.endsWith("/auth/me")).length;
  await act(async () => { held.resolve(envelope({ ...f.user, id: "next-operator" })); await refetch; });
  // The held request is the authority; no second identity response is needed to keep the route.
  expect(identities).not.toContain(null);
  expect(loginRender).not.toHaveBeenCalled();
  expect(view.router.state.location.pathname).toBe("/stacks");
  expect(client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: "next-operator" });
  expect(f.calls.filter(({ path }) => path.endsWith("/auth/me"))).toHaveLength(identityReads);
  expect(synchronousReset).toHaveBeenCalledTimes(1);
  offCache(); offIdentity(); view.unmount();
});

it("real logout/login navigates with its confirmed user before a held identity recheck and never nulls it again", async () => {
  setAppLocale("en-US");
  const held = deferred<Response>(); const recheckStarted = deferred<void>();
  let afterLogin = false;
  const f = sessionFixture((url) => {
    if (url.pathname.endsWith("/environments/0/settings")) return configured();
    if (url.pathname.endsWith("/auth/login")) { afterLogin = true; return envelope({ user: { id: "example-user", username: "operator", requiresPasswordChange: false } }); }
    if (afterLogin && url.pathname.endsWith("/auth/me")) { recheckStarted.resolve(); return held.promise; }
  });
  let client!: QueryClient;
  function Dashboard() {
    client = useQueryClient(); const [open, setOpen] = useState(false);
    return <><p>Dashboard</p><button onClick={() => setOpen(true)}>Open logout</button><LogoutDialog open={open} onOpenChange={setOpen} /></>;
  }
  const view = renderRoutes([
    { path: "/login", element: <ProductLoginPage /> },
    { path: "/", element: <AuthGate />, children: [{ path: "dashboard", element: <Dashboard /> }] },
  ], ["/dashboard"], f.adapter);
  await screen.findByText("Dashboard");
  await userEvent.click(screen.getByText("Open logout"));
  await userEvent.click(screen.getByRole("button", { name: "Got it" }));
  await screen.findByLabelText("Username");
  expect(client.getQueryData(queryKeys.currentUser)).toBeNull();
  const identities: unknown[] = [];
  const off = client.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] === queryKeys.currentUser[0]) identities.push(event.query.state.data);
  });
  await userEvent.type(screen.getByLabelText("Username"), "operator");
  await userEvent.type(screen.getByLabelText("Password"), "Example-password1!");
  await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await act(async () => { await recheckStarted.promise; });
  const firstUser = identities.findIndex((value) => !!value);
  expect(firstUser).toBeGreaterThanOrEqual(0);
  expect(identities.slice(firstUser)).not.toContain(null);
  expect(view.router.state.location.pathname).toBe("/dashboard");
  await screen.findByText("Dashboard");
  await act(async () => { held.resolve(envelope(f.user)); });
  await waitFor(() => expect(client.isFetching({ queryKey: queryKeys.currentUser })).toBe(0));
  expect(identities.slice(firstUser)).not.toContain(null);
  expect(view.router.state.location.pathname).toBe("/dashboard");
  off(); view.unmount();
});
