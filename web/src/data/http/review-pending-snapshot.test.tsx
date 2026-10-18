// Source: docklet-next/issues/85
// Revision: 0927d517e029fffe31805211b3c02e49c4a93af5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Adapted pending-snapshot regression probe and token-rotation coverage.

import { expect, it } from "vitest";
import { at, resourceFixture } from "./resources.test-support";

it.each([false, true])("A-B2 probe: a pending host still shows stale snapshot rows (Edge=%s)", async (isEdge) => {
  const f = resourceFixture();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname !== "/api/environments") return response;
    const body = await response.json();
    // Rotation sets status pending; the hub keeps the last successful snapshot for the host.
    body.data = body.data.map((row: { id: string }) => (row.id === "second" ? { ...row, status: "pending", isEdge, lastSeen: undefined, agentReport: undefined } : row));
    return new Response(JSON.stringify(body), { headers: response.headers });
  });
  f.state.reason = "agent.unavailable";
  const [stacks, containers, images, volumes, topology] = await Promise.all([f.adapter.listStacks(), f.adapter.listContainers(), f.adapter.listImages(), f.adapter.listVolumes(), f.adapter.getTopology()]);
  for (const rows of [stacks, containers, images, volumes, topology.networks]) {
    const saved = rows.filter((row) => row.hostId === "second");
    expect(saved.length).toBeGreaterThan(0);
    for (const row of saved) expect(row.inventory).toMatchObject({ stale: true, observedAt: at, reason: "agent.unavailable" });
  }
});

it.each([false, true])("A-B2 rotation retains all five lists and saved details (Edge=%s)", async (isEdge) => {
  const f = resourceFixture();
  const original = f.fetcher.getMockImplementation()!;
  let rotated = false;
  f.fetcher.mockImplementation(async (input, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/environments/second" && init?.method === "PUT") {
      expect(JSON.parse(String(init.body))).toEqual({ regenerateApiKey: true });
      rotated = true;
      f.state.reason = "agent.unavailable";
      return new Response(JSON.stringify({ success: true, data: { apiKey: "test-only" } }));
    }
    if (url.pathname.endsWith("/projects/project")) {
      const response = await original(input.toString().replace("/projects/project", "/projects"));
      const body = await response.json();
      return new Response(JSON.stringify({ success: true, data: body.data[0] }), { headers: response.headers });
    }
    const response = await original(input);
    if (url.pathname === "/api/capabilities") {
      const body = await response.json();
      for (const operationId of ["getStack", "issueAgentToken"]) body.data.push({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } });
      return new Response(JSON.stringify(body), { headers: response.headers });
    }
    if (url.pathname !== "/api/environments") return response;
    const body = await response.json();
    body.data = body.data.map((row: { id: string }) => ({ ...row, isEdge, ...(row.id === "second" && rotated ? { status: "pending", lastSeen: undefined, agentReport: undefined } : { lastSeen: at }) }));
    return new Response(JSON.stringify(body), { headers: response.headers });
  });
  const poll = () => Promise.all([f.adapter.listStacks(), f.adapter.listContainers(), f.adapter.listImages(), f.adapter.listVolumes(), f.adapter.getTopology()]);
  const before = await poll();
  expect(await f.adapter.issueAgentToken("second")).toBe("test-only");
  const host = (await f.adapter.listHosts()).find((row) => row.id === "second");
  expect(host).toMatchObject({ status: "pending", lastSeen: "", agentVersion: "" });
  const after = await poll();
  const inventories = (result: Awaited<ReturnType<typeof poll>>) => [result[0], result[1], result[2], result[3], result[4].networks];
  for (const [index, rows] of inventories(after).entries()) {
    expect(rows.map((row) => row.id)).toEqual(inventories(before)[index]!.map((row) => row.id));
    for (const row of rows.filter((row) => row.hostId === "second")) expect(row.inventory).toMatchObject({ stale: true, observedAt: at, reason: "agent.unavailable" });
  }
  for (const row of [await f.adapter.getStack("second~project"), await f.adapter.getVolume("second~data")]) {
    expect(row.hostId).toBe("second");
    expect(row.inventory).toMatchObject({ stale: true, observedAt: at, reason: "agent.unavailable" });
  }
});

it("keeps all five stale lists when a rotated direct host rejects the current token", async () => {
  const f = resourceFixture();
  f.state.status = "token-rejected";
  f.state.reason = "agent.invalid-token";
  const [stacks, containers, images, volumes, topology] = await Promise.all([f.adapter.listStacks(), f.adapter.listContainers(), f.adapter.listImages(), f.adapter.listVolumes(), f.adapter.getTopology()]);
  for (const rows of [stacks, containers, images, volumes, topology.networks]) {
    const saved = rows.filter((row) => row.hostId === "second");
    expect(saved.length).toBeGreaterThan(0);
    for (const row of saved) expect(row.inventory).toMatchObject({ stale: true, observedAt: at, reason: "agent.invalid-token" });
  }
  const host = (await f.adapter.listHosts()).find((row) => row.id === "second");
  expect(host).toMatchObject({ status: "token-rejected", inventory: { stale: true, observedAt: at, reason: "agent.invalid-token" } });
});
