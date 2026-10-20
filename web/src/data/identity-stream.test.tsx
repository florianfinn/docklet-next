// Source: docklet-next/docs/design/identity.md
// Revision: 27b31b131099aa1036d237769d18a2d03861d01c
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original authenticated activity-stream lifecycle tests.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { renderRoutes } from "@/test/render";

import { useCurrentUser } from "./queries";
import { createHttpAdapter } from "./http/http-adapter";

const json = (data: unknown, status = 200) => new Response(JSON.stringify({ success: status === 200, data }), { status, headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });

it("opens no stream on a cold login and starts after a successful sign-in", async () => {
  let signedIn = false;
  const source = { close: vi.fn(), onopen: null, onmessage: null, onerror: null };
  const eventSource = vi.fn(() => source);
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return json({ mode: "local" });
    if (path.endsWith("/settings")) return new Response("[]");
    const user = { id: "admin", username: "operator", requiresPasswordChange: false };
    if (path === "/api/auth/login") { signedIn = true; return json({ user }); }
    if (path === "/api/auth/me") return json(signedIn ? user : null, signedIn ? 200 : 401);
    return json(null, 401);
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource });
  const view = renderRoutes([{ path: "/login", element: <ProductLoginPage /> }, { path: "/dashboard", element: <p>Signed in</p> }], ["/login"], adapter);
  await screen.findByLabelText("Benutzername");
  await waitFor(() => expect(fetcher.mock.calls.some(([path]) => String(path).endsWith("/auth/me"))).toBe(true));
  expect(eventSource).toHaveBeenCalledTimes(0);
  await userEvent.type(screen.getByLabelText("Benutzername"), "operator");
  await userEvent.type(screen.getByLabelText("Passwort"), "Example-password1!");
  await userEvent.click(screen.getByRole("button", { name: "Anmelden" }));
  await screen.findByText("Signed in");
  await waitFor(() => expect(eventSource).toHaveBeenCalledTimes(1));
  view.unmount();
  expect(source.close).toHaveBeenCalledTimes(1);
});

function IdentityStatus() {
  const user = useCurrentUser();
  return <><p>{user.isSuccess ? user.data ? "Known identity" : "Rejected identity" : user.isError ? "Rejected identity" : "Pending identity"}</p><button onClick={() => void user.refetch()}>Refresh identity</button></>;
}

it("waits for identity and closes the stream when the session is rejected", async () => {
  let release: (() => void) | undefined;
  let reject = false;
  const source = { close: vi.fn(), onopen: null, onmessage: null, onerror: null };
  const eventSource = vi.fn(() => source);
  const fetcher = async (input: RequestInfo | URL) => {
    if (!String(input).endsWith("/auth/me")) return json(null);
    if (!reject) await new Promise<void>((resolve) => { release = resolve; });
    return json(reject ? null : { id: "admin", username: "operator", requiresPasswordChange: false }, reject ? 401 : 200);
  };
  const view = renderRoutes([{ path: "/", element: <IdentityStatus /> }], ["/"], createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource }));
  await screen.findByText("Pending identity");
  await waitFor(() => expect(!!release).toBe(true));
  expect(eventSource).toHaveBeenCalledTimes(0);
  await act(async () => release?.());
  await screen.findByText("Known identity");
  await waitFor(() => expect(eventSource).toHaveBeenCalledTimes(1));
  reject = true;
  await userEvent.click(screen.getByRole("button", { name: "Refresh identity" }));
  await screen.findByText("Rejected identity");
  await waitFor(() => expect(source.close).toHaveBeenCalledTimes(1));
  expect(eventSource).toHaveBeenCalledTimes(1);
  view.unmount();
});
