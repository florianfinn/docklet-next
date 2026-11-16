// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { createCsrfClient } from "./csrf-client";
import { decodeProxyOperation } from "./proxy-decoder";
import { decodeProxyData } from "./proxy-read-decoder";
import type { CreateInstance, Instance, OperationRequest, Release, Settings } from "@/features/proxy/types";

export class ProxyError extends Error {
  readonly code: string;
  readonly diagnostic?: { path?: string; line?: number; detail?: string };
  readonly status?: number;
  constructor(code: string, diagnostic?: { path?: string; line?: number; detail?: string }, status?: number) {
    super(code);
    this.code = code;
    this.diagnostic = diagnostic;
    this.status = status;
  }
}
const hub = "/api/modules/proxy";
const part = encodeURIComponent;
/** Each mounted identity owns this client and abort signal. Writes are never retried after transport loss. */
export function createProxyApi(
  signal: AbortSignal | (() => AbortSignal),
  fetcher: typeof fetch = (...args) => fetch(...args),
) {
  const client = createCsrfClient("/api", fetcher);
  async function request(path: string, method = "GET", body?: object): Promise<unknown> {
    const ownedSignal = typeof signal === "function" ? signal() : signal;
    ownedSignal.throwIfAborted();
    const response = await client.fetch(path, {
      method,
      signal: ownedSignal,
      headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    const value: unknown = await response.json();
    ownedSignal.throwIfAborted();
    if (!response.ok) {
      const p = value as { code?: string; path?: string; line?: number; detail?: string };
      throw new ProxyError(typeof p?.code === "string" ? p.code : "http.error", p, response.status);
    }
    return value;
  }
  const instancePath = (id: string) => `${hub}/instances/${part(id)}`;
  return {
    instances: async () => decodeProxyData("instances", await request(`${hub}/instances`)),
    instance: async (id: string) => decodeProxyData("instance", await request(instancePath(id))),
    discover: async (environment: string) =>
      decodeProxyData("discovery", await request(`${hub}/discover?environmentId=${part(environment)}`)),
    create: async (input: CreateInstance) =>
      decodeProxyData("instance", await request(`${hub}/instances`, "POST", input)),
    update: async (id: string, input: Settings) =>
      decodeProxyData("instance", await request(instancePath(id), "PUT", input)),
    remove: async (id: string) => {
      await request(instancePath(id), "DELETE");
    },
    inventory: async (i: Instance) =>
      decodeProxyData(
        "inventory",
        await request(`/api/environments/${part(i.environmentId)}/modules/proxy/inventory`, "POST", i),
      ),
    read: async (i: Instance) =>
      decodeProxyData(
        "read",
        await request(`/api/environments/${part(i.environmentId)}/modules/proxy/read`, "POST", i),
      ),
    operations: async (id: string) => {
      const value = (await request(`${instancePath(id)}/operations`)) as { success?: boolean; data?: unknown[] | null };
      if (value.success !== true || (value.data !== null && !Array.isArray(value.data)))
        throw new ProxyError("proxy.invalid-response");
      return (value.data ?? []).map(data => decodeProxyOperation({ success: true, data }));
    },
    operate: async (id: string, input: OperationRequest) =>
      decodeProxyOperation(await request(`${instancePath(id)}/operations`, "POST", input)),
    operation: async (id: string) => decodeProxyOperation(await request(`${hub}/operations/${part(id)}`)),
    release: async (id: string, input: Release) =>
      decodeProxyOperation(await request(`${hub}/operations/${part(id)}/release`, "POST", input)),
  };
}
export type ProxyApi = ReturnType<typeof createProxyApi>;
