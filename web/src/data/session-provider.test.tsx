// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

import { useState } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { LogoutDialog } from "@/app/shell/LogoutDialog";
import { renderRoutes } from "@/test/render";
import { sessionFixture } from "./http/session-lifecycle.test-support";
import { wireLog } from "./http/logs.test-support";
import { useCurrentUser } from "./queries";
import { queryKeys } from "./query-keys";
import { useMetrics, useStackLogs } from "./live";
import { useAdapter } from "./use-adapter";

it("the real sign-out gesture closes the activity source and leaves currentUser explicitly null", async () => {
  const f = sessionFixture(); let client!: QueryClient;
  function Page() {
    client = useQueryClient(); const user = useCurrentUser();
    return <><p>{user.data?.username ?? "No identity"}</p><LogoutDialog open onOpenChange={() => {}} /></>;
  }
  const view = renderRoutes([{ path: "/", element: <Page /> }, { path: "/login", element: <p>Login route</p> }], ["/"], f.adapter);
  await screen.findByText("operator"); await waitFor(() => expect(f.sources).toHaveLength(1));
  await userEvent.click(screen.getByRole("button", { name: "Verstanden" }));
  await screen.findByText("Login route");
  expect(client.getQueryData(queryKeys.currentUser)).toBeNull(); expect(f.sources[0]!.close).toHaveBeenCalled();
  view.unmount();
});

it("mounted live observers and paused logs clear on logout and resume for a fresh identity", async () => {
  const f = sessionFixture(); let client!: QueryClient;
  function Page() {
    const adapter = useAdapter(); client = useQueryClient(); const user = useCurrentUser();
    const [paused, pause] = useState(false);
    const lines = useStackLogs("0~example", paused);
    const history = useMetrics((value) => value?.hosts["0"]?.cpuHistory.join(",") ?? "empty");
    return <><p>{user.data?.username ?? "No identity"}</p><p data-testid="metrics">{history}</p><p data-testid="logs">{lines.map((line) => line.message).join(",")}</p>
      <button onClick={() => pause(!paused)}>Pause</button>
      <button onClick={() => void adapter.logout()}>Logout</button>
      <button onClick={() => void adapter.login("operator", "example-password").then((next) => client.setQueryData(queryKeys.currentUser, next))}>Login</button>
    </>;
  }
  const view = renderRoutes([{ path: "/", element: <Page /> }], ["/"], f.adapter);
  await screen.findByText("operator"); await waitFor(() => expect(f.sockets.length >= 2).toBe(true));
  const stats = f.sockets.find(({ url }) => url.includes("/stats"))!.socket;
  const logs = f.sockets.filter(({ url }) => url.includes("/logs")).at(-1)!.socket;
  await act(async () => { f.sample(stats); logs.send(wireLog(1,"private old tail")); });
  await waitFor(() => expect(screen.getByTestId("logs").textContent).toBe("private old tail"));
  expect(screen.getByTestId("metrics").textContent).toBe("25");
  await userEvent.click(screen.getByText("Pause")); await userEvent.click(screen.getByText("Logout"));
  await screen.findByText("No identity");
  expect(screen.getByTestId("logs").textContent).toBe(""); expect(screen.getByTestId("metrics").textContent).toBe("empty");
  expect(client.getQueryData(queryKeys.currentUser)).toBeNull();
  await userEvent.click(screen.getByText("Login")); await screen.findByText("operator");
  await userEvent.click(screen.getByText("Pause"));
  await waitFor(() => expect(f.sockets.filter(({ url }) => url.includes("/stats")).length).toBeGreaterThan(1));
  await act(async () => { f.sample(f.sockets.filter(({ url }) => url.includes("/stats")).at(-1)!.socket, 10); f.sockets.filter(({ url }) => url.includes("/logs")).at(-1)!.socket.send(wireLog(1,"fresh tail")); });
  await waitFor(() => expect(screen.getByTestId("logs").textContent).toBe("fresh tail"));
  expect(screen.getByTestId("metrics").textContent).toBe("10"); view.unmount();
});
