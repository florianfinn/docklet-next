// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createHttpAdapter } from "./http-adapter";

type Schema = components["schemas"];
export const registryDto: Schema["ContainerregistryContainerRegistry"] = { id: "saved", url: "https://registry.example.com", description: "Example", enabled: true, insecure: false, hasCredential: true, registryType: "generic", repositoryNames: [], createdAt: "2026-10-25T00:00:00Z", updatedAt: "2026-10-25T00:00:00Z" };
export const imageDto: Schema["ImageSummary"] = { id: "sha256:abc", created: 1, inUse: false, labels: {}, repo: "example/app", repoTags: ["example/app:1.0"], repoDigests: [], tag: "1.0", size: 1024, virtualSize: 1024, usageState: "known", protectionReasons: [] };
export const pruneDto: Schema["ImagePruneReport"] = { imagesDeleted: ["removed"], skipped: [{ id: "used", code: "image.in-use" }, { id: "protected", code: "image.system-protected" }], failed: [{ id: "failed", code: "image.remove-failed" }], spaceReclaimed: 0, spaceReclaimedKnown: false };
export const registryDraft = { url: registryDto.url, description: "Example", registryType: "generic" as const, enabled: true, insecure: false };
const operations = ["pullImage", "removeImage", "pruneImages", "listImages", "listHosts", "listStacks", "listContainers", "listContainerRegistries", "createContainerRegistry", "updateContainerRegistry", "clearContainerRegistryCredential", "deleteContainerRegistry", "testContainerRegistry"];
export function imageRegistryFixture(edge = false) {
  const state = { denied: new Set<string>(), calls: [] as { path: string; method: string; body: unknown }[], response: undefined as { body: unknown; status: number } | undefined, stream: '{"done":true}\n', registries: [{ ...registryDto }] as unknown[], images: [structuredClone(imageDto)], failTransport: false,
    delay: undefined as (() => Promise<void>) | undefined };
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
  const data = (value: unknown, page = false) => json({ success: true, data: value, ...(page ? { pagination: { totalItems: Array.isArray(value) ? value.length : 0 } } : {}) });
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname, method = init?.method ?? "GET";
    if (path.endsWith("/capabilities")) {
      const kind = url.searchParams.get("kind"), id = url.searchParams.get("id") ?? "";
      return data(operations.map((operationId) => ({ operationId, resourceRef: { kind, id }, available: !state.denied.has(`${operationId}:${id}`), disabledReasonCode: "capability.unavailable" })));
    }
    if (path.endsWith("/settings")) return data({ effectiveTimeZone: "UTC", settings: [] });
    if (path.endsWith("/environments")) return data([{ id: "host", name: "Example host", apiUrl: "https://agent.example.com", isEdge: edge, status: "online", enabled: true }], true);
    if (path.endsWith("/projects") || path.endsWith("/containers")) return data([], true);
    state.calls.push({ path, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (method !== "GET") {
      await state.delay?.();
      if (state.failTransport) throw new TypeError("connection lost");
      if (state.response) return json(state.response.body, state.response.status);
      if (path.endsWith("/pull")) return new Response(state.stream, { headers: { "Content-Type": "application/x-json-stream" } });
      if (path.endsWith("/prune")) return data(pruneDto);
      if (path.endsWith("/test")) { const result: Schema["ContainerregistryRegistryTestResult"] = { ok: true, checkedAt: "2026-10-25T00:00:00Z" }; return data(result); }
      if (path.includes("/container-registries") && method !== "DELETE") return data(registryDto);
      return data({ message: "Completed" });
    }
    if (path.endsWith("/container-registries")) return data(state.registries, true);
    if (path.endsWith("/images")) return data(state.images, true);
    return data({});
  });
  const source = { close: vi.fn(), onopen: null, onmessage: null, onerror: null };
  return { state, json, data, adapter: createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source }), fetcher };
}
export const mutations = (f: ReturnType<typeof imageRegistryFixture>) => f.state.calls.filter((c) => c.method !== "GET");
