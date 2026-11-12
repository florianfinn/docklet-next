// Source: docklet-next/issues/383
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-08
// License: Apache-2.0
// Deviations: Original target detail lifecycle and conflicting runtime ownership proofs.

import { act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useStack } from "../queries";
import { renderWithApp } from "@/test/render";
import type { CodedStackDetail } from "../message-code";
import { hardeningFixture } from "./web-hardening.test-support";
import { resourceFixture } from "./resources.test-support";
import { observation } from "./update-observation.test-support";

const flush = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
afterEach(() => vi.useRealTimers());

function Probe() {
  const stack = useStack("0~project");
  return <p data-testid="detail">{stack.data ? JSON.stringify(stack.data.services) : "loading"}</p>;
}

it("C1-2 target detail refreshes after lifecycle while whole-host container page two never answers", async () => {
  vi.useFakeTimers(); const f = hardeningFixture(); f.inventory.state.hostIds = ["0"];
  const original = f.fetcher.getMockImplementation()!;
  let hold = false, pageTwo = false;
  const controller = new AbortController();
  f.fetcher.mockImplementation(async (input, init) => {
    const url = new URL(String(input), "https://hub.example.com");
    const wholeContainers = hold && url.pathname.endsWith("/containers") && !url.searchParams.has("label");
    if (wholeContainers && Number(url.searchParams.get("start")) >= 100) {
      pageTwo = true;
      return new Promise<Response>((_resolve, reject) => {
        const abort = () => reject(init?.signal?.reason);
        init?.signal?.addEventListener("abort", abort, { once: true });
        if (init?.signal?.aborted) abort();
      });
    }
    const response = await original(input, init);
    if (!response.ok || !url.pathname.endsWith("/containers")) return response;
    const body = await response.json();
    body.data = body.data.map((row: Record<string, unknown>) => ({ ...row, hostConfig: { restartPolicy: "unless-stopped", memory: 268435456 } }));
    if (wholeContainers) { body.data = Array.from({ length: 100 }, (_, i) => ({ ...body.data[0], id: `other-${i}`, names: [`/other-${i}`] })); body.pagination.totalItems = 101; }
    return Response.json(body);
  });
  const view = renderWithApp(<Probe />, f.adapter);
  let pending: Promise<unknown> | undefined;
  try {
    await act(() => vi.advanceTimersByTimeAsync(100));
    expect(view.getByTestId("detail").textContent?.includes('"state":"running"')).toBe(true);
    expect(JSON.parse(view.getByTestId("detail").textContent!)[0].health).toBe("healthy");
    hold = true;
    pending = f.adapter.listContainers(undefined, controller.signal).catch(() => undefined);
    await act(flush); expect(pageTwo).toBe(true);
    await act(async () => { await f.adapter.runStackCommand("0~project", "down"); });
    f.state.stopped = true;
    f.state.run = { ...f.state.run, status: "success", endedAt: "2026-11-08T00:00:01Z" };
    act(() => f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: f.state.run.endedAt, activity: { type: "activity", activityId: f.state.run.id, activity: { ...f.state.run, resourceType: "stack" } } }), lastEventId: "" } as MessageEvent));
    await act(() => vi.advanceTimersByTimeAsync(500));
    const services = JSON.parse(view.getByTestId("detail").textContent!) as CodedStackDetail["services"];
    expect(services.map((s) => s.state)).toEqual(["stopped", "stopped"]);
    expect(services[0]).toMatchObject({ name: "web", containerId: "0~web", health: "none", image: { repository: "example", tag: "1.0.0" }, ports: [{ container: "80", host: "8080", protocol: "tcp" }], networks: ["front"], restartPolicy: "unless-stopped", memLimitMb: 256 });
    expect(f.calls.some((call) => call.includes("label=com.docker.compose.project%3Dapp"))).toBe(true);
  } finally { controller.abort(); await act(flush); await pending; view.unmount(); }
});

function ownershipFixture({ outsideLabel = false, oldPeer = false, conflict = true } = {}) {
  const f = resourceFixture(); f.state.hostIds = ["first"];
  const original = f.fetcher.getMockImplementation()!;
  const paths: string[] = [];
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname; paths.push(path + url.search);
    const response = await original(input);
    if (!response.ok) return response;
    const body = await response.json();
    if (path.endsWith("/projects")) {
      const row = { ...body.data[0], name: "display", composeProjectName: oldPeer ? undefined : "app", runtimeServices: [{ name: "web", containerId: "web", image: "example:1.0.0", status: "running" }], updateInfo: { status: "has_update", hasUpdate: true, imageCount: 1, checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0, serviceUpdates: { web: { imageRef: "example:1.0.0", updateInfo: observation("version") } } } };
      body.data = [row, ...(conflict ? [{ ...row, id: "owner", name: "owner", composeProjectName: outsideLabel ? "manual_p" : "app", runtimeServices: [] }] : [])];
      body.pagination.totalItems = body.data.length;
      // A differing observed name ties here; owner wins the stable ID ordering.
    }
    if (path.endsWith("/containers")) {
      body.data = body.data.filter((c: { id: string }) => c.id === "web");
      if (outsideLabel) body.data = body.data.map((row: { labels: Record<string, string> }) => ({ ...row, labels: { ...row.labels, "com.docker.compose.project": "manual_p" } }));
      const filter = url.searchParams.get("label");
      if (filter) body.data = body.data.filter((row: { labels: Record<string, string> }) => `com.docker.compose.project=${row.labels["com.docker.compose.project"]}` === filter);
      body.pagination.totalItems = body.data.length;
    }
    return Response.json(body, { headers: response.headers });
  });
  return { ...f, paths };
}

it("C1-2 ownership excludes a runtime entry whose concrete container belongs to a conflicting stack", async () => {
  const f = ownershipFixture();
  const stack = await f.adapter.getStack("first~project");
  expect(stack.services).toEqual([]);
  expect(f.paths.filter((path) => path.includes("/containers?")).every((path) => path.includes("label="))).toBe(true);
});

it.each([false, true])("falls back only for runtime IDs missing from the label read, old peer=%s", async (oldPeer) => {
  const f = ownershipFixture({ outsideLabel: true, oldPeer });
  expect((await f.adapter.getStack("first~project")).services).toEqual([]);
  const reads = f.paths.filter((path) => path.includes("/containers?"));
  expect(reads).toHaveLength(2); expect(reads[0]?.includes("label=")).toBe(true); expect(reads[1]?.includes("label=")).toBe(false);
});

it("retains target service update projections from the list while reading scoped container enrichment", async () => {
  const f = ownershipFixture({ conflict: false });
  const stack = await f.adapter.getStack("first~project");
  expect(stack.services).toHaveLength(1);
  expect(stack.services[0]).toMatchObject({ name: "web", containerId: "first~web", update: { state: "version" }, ports: [{ container: "80", host: "8080", protocol: "tcp" }], networks: ["front"] });
  expect(f.paths.filter((path) => path.includes("/containers?")).every((path) => path.includes("label="))).toBe(true);
});

it.each([false, true])("preserves directory-fallback enrichment without a competing owner, old peer=%s", async (oldPeer) => {
  const f = ownershipFixture({ outsideLabel: true, oldPeer, conflict: false });
  const stack = await f.adapter.getStack("first~project");
  expect(stack.services[0]).toMatchObject({ name: "web", containerId: "first~web", state: "running", health: "healthy", ports: [{ container: "80", host: "8080", protocol: "tcp" }], networks: ["front"], update: { state: "version" } });
});
