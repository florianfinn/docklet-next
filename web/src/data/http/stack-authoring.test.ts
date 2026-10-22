// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original stack authoring mapping tests for issues #138 and #139.

import { describe, expect, it, vi } from "vitest";
import { StackCreatedError } from "../adapter";
import { AdapterOutcomeError, type ResourceRef } from "../capability";
import { createStackAuthoring } from "./stack-authoring";

type Reply = Response | Error;
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 });
const input = { stackId: null, hostId: "h1", name: "notes", compose: "services:\n  app:\n    image: nginx:1.29\n", env: "TZ=UTC\n", labels: ["Web", " web", "tools"] };

function fixture(replies: Reply[], options: { denied?: string[]; up?: () => Promise<string>; labels?: string[] } = {}) {
  const fetch = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) => {
    const next = replies.shift();
    if (!next) throw new Error("unexpected request");
    if (next instanceof Error) throw next;
    return next;
  });
  const capability = vi.fn(async (operationId: string, resourceRef: ResourceRef) => ({ operationId, resourceRef, available: !options.denied?.includes(operationId) }));
  const up = vi.fn(options.up ?? (async () => "run-1"));
  const changed = vi.fn(), unauthorized = vi.fn();
  const authoring = createStackAuthoring({ baseUrl: "/api", client: { fetch: fetch as typeof globalThis.fetch, token: async () => "t" }, capability, up, labels: async () => options.labels ?? [], changed, unauthorized });
  const call = (index: number) => ({ url: String(fetch.mock.calls[index]![0]), init: fetch.mock.calls[index]![1]! });
  return { authoring, fetch, capability, up, changed, unauthorized, call };
}
const code = (error: unknown) => error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : String(error);

describe("new stack", () => {
  it("creates the project with normalized tags, then admits the guarded up", async () => {
    const f = fixture([ok({ id: "p1", name: "notes" })]);
    await expect(f.authoring.deployStack(input)).resolves.toEqual({ actionId: "run-1", stackId: "h1~p1" });
    expect(f.capability).toHaveBeenCalledWith("deployStack", { kind: "host", id: "h1" });
    const { url, init } = f.call(0);
    expect(url).toBe("/api/environments/h1/projects");
    expect(init.method).toBe("POST");
    const form = init.body as FormData;
    expect(JSON.parse(form.get("project") as string)).toEqual({ name: "notes", composeContent: input.compose, envContent: input.env, tags: ["web", "tools"] });
    expect(JSON.parse(form.get("manifest") as string)).toEqual({ fileChanges: [] });
    expect(f.up).toHaveBeenCalledWith("h1~p1");
    expect(f.changed).toHaveBeenCalled();
  });

  it("never retries a create whose response was lost", async () => {
    const f = fixture([new TypeError("network"), ok({ id: "p2" })]);
    expect(code(await f.authoring.deployStack(input).catch((e: unknown) => e))).toBe("stack.create-unknown");
    expect(f.fetch).toHaveBeenCalledTimes(1);
    expect(f.up).not.toHaveBeenCalled();
    expect(f.changed).toHaveBeenCalled();
  });

  it("reports the created stack when its first deploy is refused", async () => {
    const refusal = new AdapterOutcomeError({ kind: "conflict", code: "run.busy", activeRunId: "r0" });
    const f = fixture([ok({ id: "p1" })], { up: async () => { throw refusal; } });
    const error = await f.authoring.deployStack(input).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(StackCreatedError);
    expect((error as StackCreatedError).stackId).toBe("h1~p1");
    expect((error as StackCreatedError).cause).toBe(refusal);
    expect((error as StackCreatedError).admission).toBe("refused");
  });

  it("keeps admission uncertainty of the first deploy", async () => {
    const f = fixture([ok({ id: "p1" })], { up: async () => { throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }); } });
    const error = await f.authoring.deployStack(input).catch((e: unknown) => e) as StackCreatedError;
    expect(error).toBeInstanceOf(StackCreatedError);
    expect(error.admission).toBe("unknown");
  });

  it("rejects a malformed create response before any deploy", async () => {
    const f = fixture([ok({ id: "a~b" })]);
    expect(code(await f.authoring.deployStack(input).catch((e: unknown) => e))).toBe("stack.create-unknown");
    expect(f.up).not.toHaveBeenCalled();
  });

  it("passes hub refusals through and signals 401", async () => {
    const f = fixture([new Response(JSON.stringify({ title: "Unauthorized" }), { status: 401 })]);
    const error = await f.authoring.deployStack(input).catch((e: unknown) => e) as AdapterOutcomeError;
    expect(error.outcome).toMatchObject({ kind: "error", status: 401 });
    expect(f.unauthorized).toHaveBeenCalled();
  });

  it("does not send without capability", async () => {
    const f = fixture([], { denied: ["deployStack"] });
    const error = await f.authoring.deployStack(input).catch((e: unknown) => e) as AdapterOutcomeError;
    expect(error.outcome.kind).toBe("not-available");
    expect(f.fetch).not.toHaveBeenCalled();
  });
});

describe("existing stack", () => {
  const existing = { ...input, stackId: "h1~p1", labels: ["web", "new"] };

  it("saves files, diffs labels, then admits up in this order", async () => {
    const f = fixture([ok({}), ok({ tags: [] }), ok({ tags: [] })], { labels: ["web", "old"] });
    await expect(f.authoring.deployStack(existing)).resolves.toEqual({ actionId: "run-1", stackId: "h1~p1" });
    expect(f.capability).toHaveBeenCalledWith("deployStack", { kind: "stack", id: "h1~p1" });
    expect(f.call(0).url).toBe("/api/environments/h1/projects/p1");
    expect(f.call(0).init.method).toBe("PUT");
    expect(JSON.parse(f.call(0).init.body as string)).toEqual({ composeContent: input.compose, envContent: input.env });
    expect([1, 2].map((i) => [f.call(i).url, f.call(i).init.method, JSON.parse(f.call(i).init.body as string)])).toEqual([
      ["/api/environments/h1/projects/p1/tags", "PATCH", { name: "new", attached: true }],
      ["/api/environments/h1/projects/p1/tags", "PATCH", { name: "old", attached: false }],
    ]);
    expect(f.up.mock.invocationCallOrder[0]).toBeGreaterThan(f.fetch.mock.invocationCallOrder[2]!);
  });

  it("does not deploy when saving fails", async () => {
    const f = fixture([new Response(JSON.stringify({ title: "Bad Request", status: 400 }), { status: 400 })]);
    const error = await f.authoring.deployStack(existing).catch((e: unknown) => e) as AdapterOutcomeError;
    expect(error.outcome).toMatchObject({ kind: "error", status: 400 });
    expect(f.up).not.toHaveBeenCalled();
    expect(f.changed).toHaveBeenCalled();
  });

  it("refuses to move a stack to another host", async () => {
    const f = fixture([]);
    expect(code(await f.authoring.deployStack({ ...existing, hostId: "h2" }).catch((e: unknown) => e))).toBe("stack.host-immutable");
    expect(f.fetch).not.toHaveBeenCalled();
  });

  it("saves files alone and sends no label change for equal sets", async () => {
    const f = fixture([ok({})], { labels: ["Web"] });
    await f.authoring.saveStackFiles("h1~p1", "c", "e");
    await f.authoring.setStackLabels("h1~p1", ["web "]);
    expect(f.fetch).toHaveBeenCalledTimes(1);
    expect(f.capability).toHaveBeenCalledWith("saveStackFiles", { kind: "stack", id: "h1~p1" });
    expect(f.capability).toHaveBeenCalledWith("setStackLabels", { kind: "stack", id: "h1~p1" });
  });

  it("reports an unconfirmed save as unknown", async () => {
    const f = fixture([new Response("not json", { status: 200 })]);
    expect(code(await f.authoring.saveStackFiles("h1~p1", "c", "e").catch((e: unknown) => e))).toBe("stack.save-unknown");
  });
});

describe("docker run conversion", () => {
  it("posts the command for the host and maps the unwrapped hub response", async () => {
    const f = fixture([new Response(JSON.stringify({ success: true, dockerCompose: "services:\n  web: {}\n", envVars: "A=1", serviceName: "web" }))]);
    await expect(f.authoring.convertDockerRun("h1", "docker run nginx")).resolves.toEqual({ compose: "services:\n  web: {}\n", env: "A=1", serviceName: "web" });
    expect(f.call(0).url).toBe("/api/environments/h1/system/convert");
    expect(JSON.parse(f.call(0).init.body as string)).toEqual({ dockerRunCommand: "docker run nginx" });
    expect(f.capability).toHaveBeenCalledWith("convertDockerRun", { kind: "host", id: "h1" });
  });

  it("separates parse errors from other failures", async () => {
    const f = fixture([new Response(JSON.stringify({ status: 400 }), { status: 400 }), new Response(JSON.stringify({ status: 500 }), { status: 500 }), new TypeError("network")]);
    expect(code(await f.authoring.convertDockerRun("h1", "x").catch((e: unknown) => e))).toBe("stack.convert-invalid");
    expect(code(await f.authoring.convertDockerRun("h1", "x").catch((e: unknown) => e))).toBe("http.error");
    expect(code(await f.authoring.convertDockerRun("h1", "x").catch((e: unknown) => e))).toBe("stack.convert-failed");
  });
});

it.each([["İ", "i"], ["ΟΣ", "οσ"]])("diffs %s using hub simple lowercase %s without replacing the server tag", async (entered, server) => {
  const f = fixture([ok({}), ok({})], { labels: [server!] });
  await f.authoring.setStackLabels("h1~p1", [entered!]);
  expect(f.fetch).not.toHaveBeenCalled();
  const remove = fixture([ok({})], { labels: [server!] });
  await remove.authoring.setStackLabels("h1~p1", []);
  expect(JSON.parse(remove.call(0).init.body as string)).toEqual({ name: server, attached: false });
});
