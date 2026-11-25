// Source: docklet-next/issues/625
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 coverage: a disconnect notice turns into a reconnect notice with the same toast id.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { useHosts } from "@/data/queries";
import { renderWithApp } from "@/test/render";
import { TailnetNotifications } from "./TailnetNotifications";
import { connectedStatus, peerHost, tailnetFixture } from "./tailnet.test-support";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });

function Hosts() {
  const hosts = useHosts();
  return <><TailnetNotifications />{hosts.data?.map((host) => <span key={host.id}>{host.name}:{host.status}</span>)}</>;
}
const offline = { ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline" as const, failingLink: "peer" } };
const pending = { ...peerHost, status: "pending" };
const disconnected = "Example agent disconnected: Agent in the tailnet.";
const reconnected = "Example agent reconnected: Agent in the tailnet.";

async function start() {
  const f = tailnetFixture();
  const warning = vi.spyOn(toast, "warning").mockImplementation(() => "notice-1");
  const success = vi.spyOn(toast, "success");
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByText("Example agent:online");
  const send = async (rows: unknown[], text: string, status: Partial<ReturnType<typeof connectedStatus>> = {}) => {
    await act(async () => f.send({ ...connectedStatus(), hostCount: rows.length, ...status }, rows as never));
    if (text) await screen.findByText(text);
  };
  return { f, warning, success, send };
}

it("turns the disconnect notice into a reconnect notice with the same toast id", async () => {
  const { warning, success, send } = await start();
  expect(success).not.toHaveBeenCalled();
  await send([offline], "Example agent:offline");
  expect(warning).toHaveBeenCalledWith(disconnected);
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledWith(reconnected, { id: "notice-1" }));
  expect(success).toHaveBeenCalledTimes(1);
});

it("waits through pending until the host is reachable", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([pending], "Example agent:pending");
  expect(success).not.toHaveBeenCalled();
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledWith(reconnected, { id: "notice-1" }));
});

it("gives every interruption its own notice and success", async () => {
  const { warning, success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledTimes(1));
  warning.mockImplementation(() => "notice-2");
  await send([offline], "Example agent:offline");
  expect(warning).toHaveBeenCalledTimes(2);
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenLastCalledWith(reconnected, { id: "notice-2" }));
  expect(success).toHaveBeenCalledTimes(2);
});

it("sends no success for a host removed while disconnected", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([], "");
  await waitFor(() => expect(screen.queryByText(/Example agent:/) === null).toBe(true));
  await send([peerHost], "Example agent:online");
  expect(success).not.toHaveBeenCalled();
});

it("waits while the hub link is still failed although the host is online", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([peerHost], "Example agent:online", { connection: { state: "joining" }, routeAvailable: false } as never);
  await act(async () => { await Promise.resolve(); });
  expect(success).not.toHaveBeenCalled();
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledWith(expect.any(String), { id: "notice-1" }));
  expect(success).toHaveBeenCalledTimes(1);
});

it("waits while the peer is still offline although the host is online", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([{ ...peerHost, tailnet: { ...peerHost.tailnet!, state: "offline", failingLink: "peer" } }], "Example agent:online");
  await act(async () => { await Promise.resolve(); });
  expect(success).not.toHaveBeenCalled();
  await send([peerHost], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledWith(reconnected, { id: "notice-1" }));
});

it("gives a replacement registration no success for the old interruption", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([{ ...peerHost, registrationRevision: 2 }], "Example agent:online");
  await act(async () => { await Promise.resolve(); });
  expect(success).not.toHaveBeenCalled();
});

it("confirms the reconnect of the same registration with a new agent session", async () => {
  const { success, send } = await start();
  await send([offline], "Example agent:offline");
  await send([{ ...peerHost, edgeSessionId: "example-session-2", connectedAt: "2026-11-22T12:05:00Z" }], "Example agent:online");
  await waitFor(() => expect(success).toHaveBeenCalledWith(reconnected, { id: "notice-1" }));
});
