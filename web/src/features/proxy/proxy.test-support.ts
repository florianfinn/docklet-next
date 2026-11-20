// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { vi } from "vitest";
import { createProxyApi } from "@/data/http/proxy-api";
import type { Instance, Inventory, Operation, ReadResult, Route } from "./types";
export { Providers } from "./providers.test-support";

export const hash = "a".repeat(64),
  changedHash = "b".repeat(64);
export const instance: Instance = {
  id: "instance-one",
  environmentId: "host-one",
  name: "Example proxy",
  kind: "traefik",
  identity: { containerId: "container-one", containerName: "example-proxy", startedAt: "2026-11-16T00:00:00Z" },
  source: { kind: "project", canonical: "example-source", mountTarget: "/etc/proxy", readOnly: false },
  authority: "file",
  routeDirectory: "/etc/proxy/routes",
  generation: "generation-one",
  registration: "registration-one",
  eligibility: { writable: true, textOnly: false },
};
export const inventory: Inventory = {
  generation: instance.generation,
  identity: instance.identity,
  eligibility: instance.eligibility,
  entryPoints: [{ name: "web", address: ":80", tls: false }],
  resolvers: ["existing-resolver"],
  providers: ["file"],
  plugins: [],
  fileProvider: "/etc/proxy/routes",
  watch: true,
  throttleMillis: 2000,
  imports: [],
  adminOff: false,
  resume: false,
  persistConfig: true,
  runningDiverged: false,
  importPresent: true,
};
export const route: Route = {
  id: "routes/app.yml#app",
  name: "app",
  file: "routes/app.yml",
  class: "editable",
  eligibility: instance.eligibility,
  rule: "Host(`app.example.com`)",
  priority: 0,
  service: "app",
  serviceFile: "routes/app.yml",
  upstreams: ["http://backend.example.com:8080"],
  entryPoints: ["web"],
  references: [{ name: "headers", kind: "local", file: "routes/app.yml" }],
  dependants: ["routes/app.yml#app"],
};
export const read: ReadResult = {
  generation: instance.generation,
  inventory,
  eligibility: instance.eligibility,
  files: [
    {
      path: route.file,
      hash,
      content: "",
      mode: 420,
      modifiedAt: "2026-11-16T00:00:00Z",
      owned: true,
      lastVerifiedHash: hash,
    },
  ],
  opaqueFiles: [{ path: "routes/extra.secret.yml" }],
  routes: [route],
};
export function operation(patch: Partial<Operation> = {}): Operation {
  return {
    id: "operation-one",
    instanceId: instance.id,
    generation: instance.generation,
    registration: instance.registration,
    digest: hash,
    state: "verified",
    phase: "observing",
    evidence: "routing",
    target: route.file,
    old: { present: true, hash },
    new: { present: true, hash: changedHash },
    createdAt: "2026-11-16T00:00:00Z",
    ...patch,
  };
}
export function fixture() {
  const state = {
    instances: [structuredClone(instance)],
    inventory: structuredClone(inventory),
    read: structuredClone(read),
    operations: [] as Operation[],
    next: operation(),
    lost: false,
    operationReads: 0,
    discovery: undefined as unknown[] | undefined,
  };
  const writes: { path: string; body: unknown; method: string }[] = [];
  const json = (data: unknown) =>
    new Response(JSON.stringify({ success: true, data }), {
      status: 200,
      headers: { "Content-Type": "application/json", "X-CSRF-Token": "example" },
    });
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input),
      path = new URL(url, "https://hub.example.com").pathname,
      method = init?.method ?? "GET";
    const body = init?.body ? (JSON.parse(String(init.body)) as unknown) : undefined;
    if (method !== "GET") writes.push({ path, body, method });
    if (path.endsWith("/auth/mode")) return json({ mode: "local" });
    if (path.endsWith("/discover") && state.discovery) return json(state.discovery);
    if (path.endsWith("/discover"))
      return json([
        {
          identity: instance.identity,
          kind: "traefik",
          image: "traefik:v3.7.16",
          version: "3.7.16",
          sources: [instance.source],
        },
        {
          identity: { ...instance.identity, containerId: "assigned-container", containerName: "assigned-proxy" },
          kind: "caddy",
          image: "caddy:2.10.2",
          sources: [instance.source],
          instanceId: "configured-instance",
        },
      ]);
    if (path.endsWith("/inventory")) return json(state.inventory);
    if (path.endsWith("/read")) return json(state.read);
    if (path.endsWith("/instances") && method === "GET") return json(state.instances);
    if (path.endsWith("/instances") && method === "POST") return json({ ...instance, ...(body as object) });
    if (path.endsWith("/release")) {
      const op = operation({ state: "released" });
      state.operations = [op];
      return json(op);
    }
    if (path.endsWith("/operations") && method === "POST") {
      state.operations = [state.next];
      if (state.lost) throw new TypeError("lost-response");
      return json(state.next);
    }
    if (path.endsWith("/operations")) return json(state.operations);
    if (path.includes("/operations/")) {
      state.operationReads++;
      const recovery = state.operations.find(o => o.state === "recovery-required");
      if (recovery) return json(state.next.state === "recovery-required" ? state.next : recovery);
      state.operations = [state.next];
      return json(state.next);
    }
    if (method === "PUT") return json({ ...instance, ...(body as object) });
    if (method === "DELETE") return json(true);
    throw new Error(`unexpected-request:${method}:${path}`);
  });
  const controller = new AbortController();
  return { state, writes, fetcher, api: createProxyApi(controller.signal, fetcher), controller };
}
