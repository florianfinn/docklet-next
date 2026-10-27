// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import { AdapterOutcomeError, HUB_REF, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import type { RegistriesApi, Registry, RegistryDraft } from "@/domain/registries";
import { registryHost } from "@/domain/registries";
import type { CsrfClient } from "./csrf-client";
import { imageResponseError, record } from "./image-errors";

type Schema = components["schemas"];
/** Whitelist public fields: even a malformed peer cannot put credential fields in the query cache. */
export function mapRegistry(value: unknown): Registry {
  const row = record(value) as Partial<Schema["ContainerregistryContainerRegistry"]>;
  if (typeof row.id !== "string" || !row.id || typeof row.url !== "string" || !registryHost(row.url) || typeof row.hasCredential !== "boolean" || typeof row.enabled !== "boolean" || typeof row.insecure !== "boolean" || typeof row.registryType !== "string") throw new AdapterOutcomeError({ kind: "error", code: "registry.invalid-response" });
  return { id: row.id, url: row.url, description: typeof row.description === "string" ? row.description : "", registryType: row.registryType, enabled: row.enabled, insecure: row.insecure, hasCredential: row.hasCredential, awsRegion: row.awsRegion, repositoryNames: row.repositoryNames ?? [] };
}
export function registryWrite(input: RegistryDraft): Schema["ContainerregistryUpdateContainerRegistryRequest"] {
  return { url: input.url.trim(), description: input.description, enabled: input.enabled, insecure: input.insecure, registryType: input.registryType, awsRegion: input.awsRegion, repositoryNames: input.repositoryNames,
    ...(input.clearCredential ? { clearCredential: true } : {
      // An untouched blank credential field preserves the complete stored credential.
      ...(input.username ? { username: input.username } : {}), ...(input.token ? { token: input.token } : {}),
      ...(input.awsAccessKeyId ? { awsAccessKeyId: input.awsAccessKeyId } : {}), ...(input.awsSecretAccessKey ? { awsSecretAccessKey: input.awsSecretAccessKey } : {}),
    }) };
}
interface Options {
  baseUrl: string; client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number; changed: () => void;
}
export function createRegistryCommands({ baseUrl, client, capability, generation, changed }: Options): RegistriesApi {
  const check = (owner: number) => { if (owner !== generation()) throw new CancelledError(); };
  async function require(operation: string, id: string | undefined, owner: number) {
    const ref: ResourceRef = id ? { kind: "registry", id } : HUB_REF;
    const result = await capability(operation, ref); check(owner);
    if (!result.available) throw unavailable(operation, ref, result.disabledReasonCode);
  }
  async function request(suffix: string, method: string, input: object | undefined, owner: number, test = false) {
    let response: Response, body: unknown;
    try {
      check(owner);
      response = await client.fetch(`${baseUrl}/container-registries${suffix}`, { method, cache: "no-store", credentials: "same-origin", headers: { Accept: "application/json", ...(input ? { "Content-Type": "application/json" } : {}) }, ...(input ? { body: JSON.stringify(input) } : {}) });
      check(owner); body = await response.json(); check(owner);
    } catch (error) {
      check(owner); if (isCancelledError(error)) throw error;
      throw new AdapterOutcomeError({ kind: "error", code: method === "GET" || test ? "http.error" : "registry.outcome-unknown" });
    }
    if (!response.ok) {
      // Registry probes use a flat result body; hub authentication still follows its own error path.
      const result = record(body);
      if (test && response.status !== 401 && response.status !== 403 && result.ok === false && typeof result.checkedAt === "string" && typeof result.code === "string") return body;
      throw imageResponseError(body, response.status, false);
    }
    if (record(body).success !== true || !("data" in record(body))) throw new AdapterOutcomeError({ kind: "error", code: method === "GET" || test ? "registry.invalid-response" : "registry.outcome-unknown" });
    return test ? record(body).data : body;
  }
  async function write(operation: string, id: string | undefined, method: string, input?: RegistryDraft) {
    const owner = generation(); await require(operation, id, owner);
    if (input?.clearCredential) await require("clearContainerRegistryCredential", id, owner);
    const body = input ? registryWrite(input) : undefined;
    try { return await request(id ? `/${encodeURIComponent(id)}` : "", method, body, owner); }
    finally { if (owner === generation()) changed(); }
  }
  return {
    async listContainerRegistries() {
      const owner = generation(); await require("listContainerRegistries", undefined, owner);
      const result: Registry[] = []; let start = 0;
      for (;;) {
        const body = record(await request(`?start=${start}&limit=100`, "GET", undefined, owner));
        if (body.data !== null && !Array.isArray(body.data)) throw imageResponseError(null);
        const rows = (body.data ?? []) as unknown[];
        const total = record(body.pagination).totalItems;
        if (typeof total !== "number" || !Number.isInteger(total) || total < 0 || (!rows.length && start < total)) throw imageResponseError(null);
        result.push(...rows.map(mapRegistry)); start += rows.length;
        if (start >= total) return result;
      }
    },
    async createContainerRegistry(input) { return mapRegistry(record(await write("createContainerRegistry", undefined, "POST", input)).data); },
    async updateContainerRegistry(id, input) { return mapRegistry(record(await write("updateContainerRegistry", id, "PUT", input)).data); },
    async deleteContainerRegistry(id) { await write("deleteContainerRegistry", id, "DELETE"); },
    async testContainerRegistry(id) {
      const owner = generation(); await require("testContainerRegistry", id, owner);
      const value = record(await request(`/${encodeURIComponent(id)}/test`, "POST", undefined, owner, true));
      if (typeof value.ok !== "boolean" || typeof value.checkedAt !== "string" || (!value.ok && typeof value.code !== "string")) throw imageResponseError(null);
      return { ok: value.ok, checkedAt: value.checkedAt, ...(typeof value.code === "string" ? { code: value.code } : {}) };
    },
  };
}
