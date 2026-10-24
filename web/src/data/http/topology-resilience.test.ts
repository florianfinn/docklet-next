// Source: docklet-next/issues/204
// Revision: def05d5f7606c94c26f8fdeb1b16c53c473a0c11
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original Direct and Edge partial topology regression matrix.

import { describe, expect, it } from "vitest";
import type { Topology } from "@/domain/model";
import { barrier, topologyFixture } from "./topology-resilience.test-support";

const ids = ["first~good", "first~missing", "second~good", "second~missing"];
describe.each([{ transport: "Direct", isEdge: false }, { transport: "Edge", isEdge: true }])("$transport partial topology", ({ isEdge }) => {
  it("keeps listed identity and successful details after one initial inspect fails", async () => {
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/networks/missing");
    const graph = await f.adapter.getTopology();
    expect(graph.networks.map((n) => n.id)).toEqual(ids);
    expect(graph.networks.find((n) => n.id === "second~missing")).toMatchObject({ name: "unavailable", detailUnavailable: true, inventory: { stale: true, reason: "inventory.read-error" } });
    expect(graph.networks.find((n) => n.id === "second~good")).toMatchObject({ internal: true, ipv6: true, inventory: { stale: false } });
    expect(graph.networks.find((n) => n.id === "second~good")?.detailUnavailable).toBeUndefined();
    expect(graph.inventory?.second).toMatchObject({ stale: true, reason: "inventory.read-error" });
    expect(graph.inventory?.first).toBeUndefined();
  });

  it("keeps all list rows when every inspect on one host fails", async () => {
    const f = topologyFixture(isEdge);
    for (const id of ["good", "missing"]) f.failures.add(`/api/environments/second/networks/${id}`);
    const graph = await f.adapter.getTopology();
    expect(graph.networks.map((n) => n.id)).toEqual(ids);
    expect(graph.networks.filter((n) => n.detailUnavailable).map((n) => n.hostId)).toEqual(["second", "second"]);
    expect(graph.networks.filter((n) => n.hostId === "first").every((n) => !n.inventory?.stale)).toBe(true);
  });

  it.each(["online", "pending"])("retains the query snapshot when a %s host list fails, then replaces it on recovery", async (status) => {
    const f = topologyFixture(isEdge);
    const retained = await f.adapter.getTopology();
    f.state.status = status;
    f.failures.add("/api/environments/second/networks");
    const partial = await f.adapter.getTopology(undefined, retained);
    expect(partial.networks.map((n) => n.id)).toEqual(ids);
    expect(partial.networks.filter((n) => n.hostId === "second").every((n) => n.detailUnavailable && n.inventory?.stale)).toBe(true);
    expect(partial.inventory?.second?.reason).toBe("inventory.read-error");
    f.failures.clear();
    f.state.networks = [f.state.networks[0]!];
    const recovered = await f.adapter.getTopology(undefined, partial);
    expect(recovered.networks.map((n) => n.id)).toEqual(["first~good", "second~good"]);
    expect(recovered.inventory).toBeUndefined();
    expect(recovered.networks.every((n) => !n.detailUnavailable && !n.inventory?.stale)).toBe(true);
  });

  it.each(["first", "second"])("preserves both hosts while %s is delayed during refetch", async (slow) => {
    const f = topologyFixture(isEdge);
    const previous = await f.adapter.getTopology();
    const hold = f.delay(`/api/environments/${slow}/networks`);
    const batches: Topology[] = [];
    const published = barrier();
    const pending = f.adapter.getTopology((batch) => { batches.push(batch); published.release(); }, previous);
    await hold.entered.promise;
    // Await the actual other-host publication, rather than a timer.
    await published.promise;
    expect(batches[0]?.networks.map((n) => n.id)).toEqual(ids);
    hold.done.release();
    expect((await pending).networks.map((n) => n.id)).toEqual(ids);
    expect(batches.every((batch) => batch.networks.length === 4)).toBe(true);
  });

  it("clears per-network and host errors after successful inspect recovery", async () => {
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/networks/missing");
    const partial = await f.adapter.getTopology();
    f.failures.clear();
    const recovered = await f.adapter.getTopology(undefined, partial);
    expect(recovered.inventory).toBeUndefined();
    expect(recovered.networks.every((n) => !n.detailUnavailable && !n.inventory?.stale)).toBe(true);
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.stale).toBe(false);
  });

  it("keeps listed identity when membership context fails", async () => {
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/containers");
    const graph = await f.adapter.getTopology();
    expect(graph.networks.map((n) => n.id)).toEqual(ids);
    expect(graph.networks.filter((n) => n.hostId === "second").every((n) => n.detailUnavailable && n.inventory?.stale)).toBe(true);
  });
  it("keeps successful inspect observations when another detail read fails", async () => {
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/networks/missing");
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => {
      const response = await original(input);
      if (String(input).endsWith("/second/networks/good")) {
        response.headers.set("X-Docklet-Observed-At", "2026-10-16T12:00:00Z");
        response.headers.set("X-Docklet-Host-Reason", "agent.unavailable");
      }
      return response;
    });
    const graph = await f.adapter.getTopology();
    expect(graph.networks.find((n) => n.id === "second~good")?.inventory).toMatchObject({ observedAt: "2026-10-16T12:00:00Z", stale: true, reason: "agent.unavailable" });
    expect(graph.networks.find((n) => n.id === "first~good")?.inventory?.stale).toBe(false);
  });

  it("keeps list identities when one inspect detail cannot be mapped", async () => {
    const f = topologyFixture(isEdge);
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => {
      const response = await original(input);
      if (!String(input).endsWith("/second/networks/missing")) return response;
      const body = await response.json();
      body.data.driver = "";
      return new Response(JSON.stringify(body), { headers: response.headers });
    });
    const graph = await f.adapter.getTopology();
    expect(graph.networks.map((n) => n.id)).toEqual(ids);
    expect(graph.networks.find((n) => n.id === "second~missing")).toMatchObject({ detailUnavailable: true, inventory: { reason: "inventory.mapping-error", stale: true } });
    expect(graph.networks.find((n) => n.id === "second~good")?.inventory?.stale).toBe(false);
  });

});
