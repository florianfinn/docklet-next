// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { describe, expect, it, vi } from "vitest";
import { AdapterOutcomeError } from "@/data/capability";
import { imageRegistryFixture, mutations, pruneDto } from "./image-registry.test-support";
import { imageResponseError } from "./image-errors";
import type { components } from "@/api/generated/schema";

const code = (error: unknown) => error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
describe.each([false, true])("image writes through the hub (Edge=%s)", (edge) => {
  it("maps pull reference only and accepts completion without an activity frame", async () => {
    const f = imageRegistryFixture(edge);
    expect((await f.adapter.listHosts())[0]?.connection).toBe(edge ? "edge" : "direct");
    expect(await f.adapter.pullImage({ hostId: "host", reference: "registry.example.com/team/app:1" })).toEqual({ actionId: undefined });
    expect(mutations(f)[0]).toEqual({ path: "/api/environments/host/images/pull", method: "POST", body: { imageName: "registry.example.com/team/app:1" } });
  });
  it("preserves the optional real activity identity and maps streamed rejection", async () => {
    const f = imageRegistryFixture(edge);
    const started: components["schemas"]["ImagePullStartedFrame"] = { type: "activity", activityId: "action" };
    f.state.stream = `${JSON.stringify(started)}\n{"status":"Downloading","id":"abcdef123456"}\n{"done":true}\n`;
    expect(await f.adapter.pullImage({ hostId: "host", reference: "example:1" })).toEqual({ actionId: "action" });
    f.state.stream = `${JSON.stringify(started)}\n{"error":{"code":"image.busy","message":"never rendered"}}\n`;
    const failure = await f.adapter.pullImage({ hostId: "host", reference: "example:1" }).catch((e: unknown) => e);
    expect([code(failure), (failure as { actionId: string }).actionId]).toEqual(["image.busy", "action"]);
  });
  it.each(["", '{"type":"activity","activityId":"action"}\n', '{"status":"Waiting"}\n', '{"done":false}\n', '{"done":true}\n{"status":"Waiting"}\n', 'invalid'])('keeps incomplete or malformed stream outcome unknown (%s)', async (stream) => {
    const f = imageRegistryFixture(edge); f.state.stream = stream;
    expect(await f.adapter.pullImage({ hostId: "host", reference: "example:1" }).catch(code)).toBe("image.outcome-unknown");
  });
  it("requires exact image/host capabilities before every write", async () => {
    const f = imageRegistryFixture(edge);
    f.state.denied.add("removeImage:host~sha256:abc"); f.state.denied.add("pullImage:host"); f.state.denied.add("pruneImages:host");
    await expect(f.adapter.removeImage("host~sha256:abc", true)).rejects.toMatchObject({ outcome: { kind: "not-available", resourceRef: { kind: "image", id: "host~sha256:abc" } } });
    await expect(f.adapter.pullImage({ hostId: "host", reference: "example:1" })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.pruneImages({ hostIds: ["other", "host"], mode: "unused" })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(mutations(f)).toEqual([]);
  });
  it("maps force and nested typed rejection without message parsing", async () => {
    const f = imageRegistryFixture(edge);
    f.state.response = { status: 409, body: { success: false, error: { code: "image.in-use", message: "untrusted" } } satisfies components["schemas"]["ImageOperationError"] };
    expect(await f.adapter.removeImage("host~sha256:abc", true).catch(code)).toBe("image.in-use");
    expect(mutations(f)[0]?.path).toBe("/api/environments/host/images/sha256%3Aabc");
  });
  it.each([502, 504])("classifies proxy %s as unknown and invalidates images", async (status) => {
    const f = imageRegistryFixture(edge), changed = vi.fn(), stop = f.adapter.subscribeChanges(changed);
    f.state.response = { status, body: { code: "agent.unavailable" } };
    expect(await f.adapter.removeImage("host~image").catch(code)).toBe("image.outcome-unknown");
    expect(changed).toHaveBeenCalledWith(["images"]); stop();
  });
  it("returns the synchronous prune report with skipped and failed IDs; no invented reclaimed space", async () => {
    const f = imageRegistryFixture(edge);
    const result = await f.adapter.pruneImages({ hostIds: ["host", "host"], mode: "unused" });
    expect(result.hosts).toEqual([{ hostId: "host", removed: ["host~removed"], skipped: pruneDto.skipped!.map((i) => ({ ...i, id: `host~${i.id}` })), failed: [{ id: "host~failed", code: "image.remove-failed" }] }]);
    expect(mutations(f)).toHaveLength(1); expect(mutations(f)[0]?.body).toEqual({ mode: "all" });
  });
  it("preserves successful hosts beside unknown prune results", async () => {
    const f = imageRegistryFixture(edge), original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init) => String(input).includes("/other/images/prune") ? f.json({ code: "agent.unavailable" }, 502) : original(input, init));
    const result = await f.adapter.pruneImages({ hostIds: ["host", "other"], mode: "dangling" });
    expect(result.hosts[0]?.removed).toEqual(["host~removed"]); expect(result.hosts[1]?.errorCode).toBe("image.outcome-unknown");
  });
});
it("never infers a code from free-form message text", () => {
  expect(code(imageResponseError({ message: "image.in-use", detail: "image.system-protected" }, 500))).toBe("http.error");
});

it("discards late writes and publications after identity change", async () => {
  const { createImageCommands } = await import("./image-commands");
  let generation = 0, release!: (response: Response) => void;
  const fetcher = vi.fn(() => new Promise<Response>((resolve) => { release = resolve; }));
  const changed = vi.fn();
  const commands = createImageCommands({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, generation: () => generation, changed,
    capability: async (operationId, resourceRef) => ({ available: true, operationId, resourceRef }) });
  const pending = commands.removeImage("host~image");
  await vi.waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
  generation++;
  release(new Response(JSON.stringify({ success: true, data: { message: "Completed" } })));
  await expect(pending).rejects.toMatchObject({ revert: undefined });
  expect(changed).not.toHaveBeenCalled();
});
it("maps only safe generated progress phases and counts, never raw status prose", async () => {
  const f = imageRegistryFixture(), progress = vi.fn();
  f.state.stream = '{"status":"Downloading","id":"abcdef123456","progressDetail":{"current":5,"total":10}}\n{"done":true}\n';
  await f.adapter.pullImage({ hostId: "host", reference: "example:1", onProgress: progress });
  expect(progress).toHaveBeenCalledExactlyOnceWith({ phase: "downloading", current: 5, total: 10 });
  f.state.stream = '{"status":"untrusted registry response"}\n{"done":true}\n';
  expect(await f.adapter.pullImage({ hostId: "host", reference: "example:1", onProgress: progress }).catch(code)).toBe("image.outcome-unknown");
  expect(progress).toHaveBeenCalledTimes(1);
});

it("accepts a single large network chunk containing many complete valid frames", async () => {
  const f = imageRegistryFixture(), progress = vi.fn();
  const line = JSON.stringify({ status: "Downloading", id: "abcdef123456", progressDetail: { current: 5, total: 10 } });
  f.state.stream = `${line}\n`.repeat(1000) + '{"done":true}\n';
  expect(new TextEncoder().encode(f.state.stream).length).toBeGreaterThan(65536);
  await expect(f.adapter.pullImage({ hostId: "host", reference: "example:1", onProgress: progress })).resolves.toEqual({ actionId: undefined });
  expect(progress).toHaveBeenCalledTimes(1000);
});
it("rejects an oversized unfinished line before a later chunk can complete it", async () => {
  const f = imageRegistryFixture(), progress = vi.fn(), cancel = vi.fn();
  const original = f.fetcher.getMockImplementation()!;
  const chunks = ['{"status":"Waiting"}\n' + " ".repeat(65537) + '{"status":"Waiting"', '}\n{"done":true}\n'];
  f.fetcher.mockImplementation(async (input, init) => String(input).endsWith("/pull") ? new Response(new ReadableStream({
    start(controller) { for (const chunk of chunks) controller.enqueue(new TextEncoder().encode(chunk)); controller.close(); }, cancel,
  })) : original(input, init));
  expect(await f.adapter.pullImage({ hostId: "host", reference: "example:1", onProgress: progress }).catch(code)).toBe("image.outcome-unknown");
  expect(progress).toHaveBeenCalledExactlyOnceWith({ phase: "waiting" });
  expect(cancel).toHaveBeenCalledOnce();
});
