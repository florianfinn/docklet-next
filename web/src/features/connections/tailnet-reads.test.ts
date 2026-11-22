// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original late-read and identity ownership proofs for tailnet observations.

import { CancelledError } from "@tanstack/react-query";
import { expect, it } from "vitest";
import { createCsrfClient } from "@/data/http/csrf-client";
import type { TailnetStatus } from "@/domain/tailnet";
import { createTailnet } from "@/data/http/tailnet";
import { connectedStatus, peerHost } from "./tailnet.test-support";

function heldRead() {
  let finish!: (response: Response) => void;
  let identity = 1;
  const controller = createTailnet("/api", { token: async () => "example", fetch: () => new Promise<Response>((resolve) => { finish = resolve; }) }, () => identity);
  return { controller, replaceIdentity: () => { identity++; controller.clear(); }, answer: (status: TailnetStatus = { ...connectedStatus(), connection: { state: "joining" } }) => finish(new Response(JSON.stringify({ success: true, data: status }))) };
}

it("does not replace a newer stream observation with an earlier read of the same generation", async () => {
  const f = heldRead(); const read = f.controller.api.status();
  f.controller.observe(connectedStatus()); f.answer(); await read;
  expect(f.controller.api.current()?.connection.state).toBe("connected");
});

it("cancels the result of a status read held across identity replacement", async () => {
  const f = heldRead(); const read = f.controller.api.status();
  f.replaceIdentity(); f.answer();
  await expect(read).rejects.toBeInstanceOf(CancelledError);
  expect(f.controller.api.current()).toBeUndefined();
});


it("never sends a credential write queued across an identity replacement", async () => {
  let release!: (response: Response) => void;
  let identity = 1, writes = 0;
  const client = createCsrfClient("/api", async (_input, init) => {
    if (init?.method === "PUT") { writes++; return new Response(JSON.stringify({ success: true, data: {} })); }
    return new Promise<Response>((resolve) => { release = resolve; });
  });
  const controller = createTailnet("/api", client, () => identity);
  const save = controller.api.save({ enabled: true, mode: "oauth", clientId: "example", clientSecret: "example" });
  ++identity; controller.clear();
  release(new Response("{}", { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } }));
  await expect(save).rejects.toBeInstanceOf(CancelledError);
  expect(writes).toBe(0);
});


it("never attaches a streamed peer observation to a different registration revision", () => {
  const f = heldRead();
  f.controller.observeHosts([peerHost]);
  const replacement = { ...peerHost, registrationRevision: 2, tailnet: undefined };
  expect(f.controller.project(replacement).tailnet).toBeUndefined();
});


it("accepts lower process generations after a stream reopen and keeps fencing within the new process", async () => {
  const f = heldRead();
  f.controller.observe({ ...connectedStatus(), generation: 9 });
  const reopened = f.controller.reconnect();
  f.answer({ ...connectedStatus(), generation: 1, enabled: false, connection: { state: "off" }, routeAvailable: false });
  await reopened;
  expect(f.controller.api.current()?.connection.state).toBe("off");
  expect(f.controller.api.current()?.generation).toBe(1);
  f.controller.observe({ ...connectedStatus(), generation: 0 });
  expect(f.controller.api.current()?.connection.state).toBe("off");
});

it("does not invent an old-process generation for a write spanning a stream reopen", async () => {
  let release!: (response: Response) => void;
  let status = { ...connectedStatus(), generation: 1 };
  const controller = createTailnet("/api", { token: async () => "example", fetch: async (_input, init) => {
    if (init?.method === "PUT") return new Promise<Response>((resolve) => { release = resolve; });
    return new Response(JSON.stringify({ success: true, data: status }));
  } }, () => 1);
  controller.observe({ ...connectedStatus(), generation: 9 });
  const save = controller.api.save({ enabled: false });
  await controller.reconnect();
  status = { ...connectedStatus(), generation: 2, enabled: false, connection: { state: "off" }, routeAvailable: false };
  release(new Response(JSON.stringify({ success: true, data: { enabled: false } })));
  await save;
  controller.observe({ ...connectedStatus(), generation: 3 });
  expect(controller.api.current()?.connection.state).toBe("connected");
  expect(controller.api.current()?.generation).toBe(3);
});
