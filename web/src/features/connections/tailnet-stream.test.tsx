// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { useHosts } from "@/data/queries";
import { mapHost } from "@/data/http/hosts";
import { tailnetChain } from "@/domain/tailnet";
import { createTailnet } from "@/data/http/tailnet";
import { renderWithApp } from "@/test/render";
import { HostCard } from "@/features/hosts/HostCard";
import { TailnetHeaderChip } from "./TailnetHosts";
import { TailnetNotifications } from "./TailnetNotifications";
import { connectedStatus, peerHost, tailnetFixture } from "./tailnet.test-support";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });
function Hosts() {
  const hosts = useHosts();
  return <><TailnetHeaderChip /><TailnetNotifications />{hosts.data?.map((host) => <HostCard key={host.id} host={host} system={undefined} allIds={[host.id]} />)}</>;
}

it("streams the producer peer projection through chip, badge, dialog, tooltip and offline notification without reload", async () => {
  const f = tailnetFixture();
  const notify = vi.spyOn(toast, "warning");
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByRole("link", { name: /Tailscale: Connected · 1 host/ });
  await screen.findByText("Example agent");
  const rows = [{ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline" as const, failingLink: "peer" } }];
  await act(async () => f.send({ ...connectedStatus(), hostCount: 2, onlineHosts: 0 }, rows));
  await screen.findByRole("link", { name: /Tailscale: Connected · 2 hosts/ });
  await screen.findByRole("button", { name: "Show cause" });
  expect(screen.getByText("Tailscale").closest('[data-slot="badge"]') !== null).toBe(true);
  expect(notify).toHaveBeenCalledWith("Example agent disconnected: Agent in the tailnet.");
  await userEvent.click(screen.getByRole("button", { name: "Show cause" }));
  const dialog = await screen.findByRole("dialog");
  expect([...dialog.querySelectorAll("[data-state][data-link]")].map((el) => [el.getAttribute("data-link"), el.getAttribute("data-state")])).toEqual([["hub", "ok"], ["peer", "failed"], ["tunnel", "unknown"]]);
  expect(within(dialog).queryByText("Via relay") !== null).toBe(true);
  await userEvent.keyboard("{Escape}");
  const status = screen.getByText("Disconnected", { exact: true });
  await userEvent.hover(status.closest('[tabindex="0"]')!);
  const tooltip = await screen.findByRole("tooltip");
  expect(tooltip.querySelector('[data-link="peer"]')?.getAttribute("data-state")).toBe("failed");
  expect(tooltip.querySelector('[data-link="tunnel"]')?.getAttribute("data-state")).toBe("unknown");
  // One existing catalogue refresh follows the stream; all consumers share that query.
  expect(f.requests.filter((path) => path === "/api/environments")).toHaveLength(2);
});

it("keeps connection and link 1 truthful when provisioning is rejected", async () => {
  const f = tailnetFixture(); const notify = vi.spyOn(toast, "warning");
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByRole("link", { name: /Tailscale: Connected/ });
  await act(async () => f.send({ ...connectedStatus(), provisioning: { state: "rejected", code: "tailnet.credential-rejected", providerStatus: 403 }, routeAvailable: false, routeCode: "tailnet.credential-rejected" }));
  expect(screen.getByRole("link", { name: /Tailscale: Connected/ }) !== null).toBe(true);
  expect(tailnetChain(mapHost(peerHost), f.adapter.tailnet!.current()).map((row) => row.state)).toEqual(["ok", "ok", "ok"]);
  expect(notify).not.toHaveBeenCalled();
});

it.each(["disable", "reset"])("drops a late connected generation after %s", async (action) => {
  const f = tailnetFixture(); await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(() => {});
  await f.adapter.tailnet!.status();
  if (action === "disable") await f.adapter.tailnet!.save({ enabled: false }); else await f.adapter.tailnet!.reset();
  f.send(connectedStatus());
  expect(f.adapter.tailnet!.current()?.connection.state).toBe("off");
  expect(f.adapter.tailnet!.current()?.generation).toBe(2);
  stop();
});

it("refreshes an omitted hub status after another browser disables the route", async () => {
  const f = tailnetFixture(); renderWithApp(<Hosts />, f.adapter);
  await screen.findByRole("link", { name: /Tailscale: Connected/ });
  f.state.status = { ...connectedStatus(), enabled: false, generation: 2, connection: { state: "off" }, routeAvailable: false };
  await act(async () => f.send(undefined));
  await waitFor(() => expect(screen.queryByRole("link", { name: /Tailscale:/ }) === null).toBe(true));
});

it.each([
  ["hub", "off", "offline", "offline", ["failed", "unknown", "unknown"]],
  ["peer", "connected", "offline", "offline", ["ok", "failed", "unknown"]],
  ["tunnel", "connected", "online", "offline", ["ok", "ok", "failed"]],
  ["standby", "connected", "online", "standby", ["ok", "ok", "ok"]],
  // Parity with backend tailnet.FailingLink: an unknown peer is no failure; only offline/error/token-rejected hosts fail the tunnel.
  ["unknown peer, offline host", "connected", "unknown", "offline", ["ok", "ok", "failed"]],
  ["unknown peer, pending host", "connected", "unknown", "pending", ["ok", "ok", "ok"]],
  ["update required", "connected", "online", "update-required", ["ok", "ok", "ok"]],
  ["token rejected", "connected", "online", "token-rejected", ["ok", "ok", "failed"]],
  ["error host", "connected", "online", "error", ["ok", "ok", "failed"]],
  ["hub before unknown peer", "joining", "unknown", "pending", ["failed", "unknown", "unknown"]],
] as const)("orders the offline chain for %s", (_name, connection, peer, tunnel, states) => {
  expect(tailnetChain(mapHost({ ...peerHost, status: tunnel, tailnet: { ...peerHost.tailnet!, state: peer } }), { ...connectedStatus(), connection: { state: connection } }).map((row) => row.state)).toEqual(states);
});

it("clears hub observations on identity replacement", async () => {
  const f = tailnetFixture(); await f.adapter.getCurrentUser(); await f.adapter.tailnet!.status();
  expect(f.adapter.tailnet!.current()?.enabled).toBe(true);
  f.state.admin = false; await f.adapter.getCurrentUser();
  expect(f.adapter.tailnet!.current()).toBeUndefined();
});


it("uses the producer route and peer projection when the HTTP catalogue has older peer fields", async () => {
  const f = tailnetFixture();
  const old = { ...peerHost, networkRoute: undefined, tailnet: undefined };
  f.state.rows = [old];
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByText("Example agent");
  await screen.findByRole("link", { name: /Tailscale: Connected/ });
  expect(screen.queryByText("Tailscale") === null).toBe(true);
  await act(async () => {
    f.send(connectedStatus(), [{ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline", path: "direct", failingLink: "peer" } }]);
    f.state.rows = [{ ...old, status: "offline" }];
  });
  await screen.findByText("Disconnected", { exact: true });
  expect(screen.queryByText("Tailscale") !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Show cause" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.querySelector('[data-link="peer"]')?.getAttribute("data-state")).toBe("failed");
  expect(within(dialog).queryByText("Direct") !== null).toBe(true);
});


it("reseeds the admin observation when the actual SSE source reopens after a hub restart", async () => {
  const f = tailnetFixture({ ...connectedStatus(), generation: 9 });
  await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeChanges(() => {});
  f.sources.at(-1)!.onopen?.();
  await f.adapter.tailnet!.status();
  expect(f.adapter.tailnet!.current()?.generation).toBe(9);
  f.state.status = { ...connectedStatus(), generation: 1, enabled: false, connection: { state: "off" }, routeAvailable: false };
  f.sources.at(-1)!.onopen?.();
  await f.adapter.tailnet!.status();
  expect(f.adapter.tailnet!.current()?.connection.state).toBe("off");
  expect(f.adapter.tailnet!.current()?.generation).toBe(1);
  stop();
});

it("keeps a connected run truthful after a same-mode credential save and keeps streaming hosts", async () => {
  const f = tailnetFixture(); renderWithApp(<Hosts />, f.adapter);
  await screen.findByRole("link", { name: /Tailscale: Connected · 1 host/ });
  await act(async () => { await f.adapter.tailnet!.save({ enabled: true, mode: "oauth", clientId: "example-client", clientSecret: "test-only" }); });
  expect(f.adapter.tailnet!.current()?.connection.state).toBe("connected");
  expect(f.adapter.tailnet!.current()?.generation).toBe(1);
  const rows = [{ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline" as const, failingLink: "peer" } }];
  await act(async () => f.send(connectedStatus(), rows));
  await screen.findByRole("button", { name: "Show cause" });
  expect(screen.getByRole("link", { name: /Tailscale: Connected/ }) !== null).toBe(true);
});

it("applies the host rows of an envelope whose hub status is stale", async () => {
  const f = tailnetFixture({ ...connectedStatus(), generation: 3 }); renderWithApp(<Hosts />, f.adapter);
  await screen.findByRole("link", { name: /Tailscale: Connected · 1 host/ });
  const rows = [{ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "offline" as const, failingLink: "peer" } }];
  await act(async () => f.send({ ...connectedStatus(), generation: 2, connection: { state: "joining" } }, rows));
  await screen.findByRole("button", { name: "Show cause" });
  expect(f.adapter.tailnet!.current()?.generation).toBe(3);
});

it("shows no cause for a new tailnet host that waits for its agent", async () => {
  const f = tailnetFixture(); const notify = vi.spyOn(toast, "warning");
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByText("Example agent");
  const waiting = { ...peerHost, status: "pending", tailnet: { ...peerHost.tailnet!, state: "unknown" as const, failingLink: "none" } };
  await act(async () => f.send(connectedStatus(), [waiting]));
  await act(async () => f.send(connectedStatus(), [{ ...waiting, tailnet: { ...waiting.tailnet, state: "online" as const } }]));
  await waitFor(() => expect(f.requests.filter((path) => path === "/api/environments").length >= 3).toBe(true));
  expect(screen.queryByRole("button", { name: "Show cause" }) === null).toBe(true);
  expect(notify).not.toHaveBeenCalled();
});

it("fences each overlapping credential save until its own status read settles", async () => {
  const pending: ((response: Response) => void)[] = [];
  const fetcher = vi.fn(() => new Promise<Response>((resolve) => pending.push(resolve)));
  const tailnet = createTailnet("/api", { fetch: fetcher, token: async () => "" } as never, () => 0);
  tailnet.observe(connectedStatus());
  const reply = (index: number, data: unknown) => pending[index]!(new Response(JSON.stringify({ success: true, data })));
  const first = tailnet.api.save({ enabled: true });
  reply(0, {}); await vi.waitFor(() => expect(pending).toHaveLength(2));
  // The first PUT settled; its status read is still in flight while a second save completes.
  const second = tailnet.api.save({ enabled: true });
  reply(2, {}); await vi.waitFor(() => expect(pending).toHaveLength(4));
  reply(3, connectedStatus()); await second;
  expect(tailnet.observe({ ...connectedStatus(), connection: { state: "joining" } })).toBe(false);
  expect(tailnet.api.current()?.connection.state).toBe("connected");
  reply(1, connectedStatus()); await first;
  expect(tailnet.observe({ ...connectedStatus(), connection: { state: "joining" } })).toBe(true);
});

it("lets a catalogue read after the latest frame show the newer peer state", async () => {
  const f = tailnetFixture(); const notify = vi.spyOn(toast, "warning");
  renderWithApp(<Hosts />, f.adapter);
  await screen.findByText("Example agent");
  // Diagnosis timeline: the delayed frame still carries the peer online; the refetch it triggers already reads the peer offline.
  const sessionGone = { ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "online" as const, failingLink: "tunnel" } };
  await act(async () => {
    f.send(connectedStatus(), [sessionGone]);
    f.state.rows = [{ ...sessionGone, tailnet: { ...sessionGone.tailnet, state: "offline", path: "unknown", failingLink: "peer" } }];
  });
  await userEvent.click(await screen.findByRole("button", { name: "Show cause" }));
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect(dialog.querySelector('[data-link="peer"]')?.getAttribute("data-state")).toBe("failed"));
  expect(notify).toHaveBeenLastCalledWith("Example agent disconnected: Agent in the tailnet.");
});
