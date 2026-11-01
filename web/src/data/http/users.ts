// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original user administration and personal-key HTTP mappings.

import type { components } from "@/api/generated/schema";
import type { ManagedUser, PersonalApiKey, UsersApi } from "../api";
import { AdapterOutcomeError } from "../capability";
import type { CsrfClient } from "./csrf-client";

export const USER_OPERATIONS = ["listUsers", "createUser", "updateUser", "disableUser", "enableUser", "resetUserPassword", "deleteUser", "setUserAdministrator", "setUserAppAssignments", "listMyApiKeys", "createMyApiKey", "deleteMyApiKey", "getAuthConfiguration"];

export function mapManagedUser(row: components["schemas"]["UserUser"]): ManagedUser {
  if (!row || typeof row.id !== "string" || typeof row.username !== "string" || typeof row.isAdmin !== "boolean") throw new Error("users.invalid-response");
  return { id: row.id, username: row.username, isAdmin: row.isAdmin, disabledAt: row.disabledAt, requiresPasswordChange: row.requiresPasswordChange, assignments: row.assignments ?? [] };
}

/** Explicit projection prevents a creation secret entering list/query state. */
export function mapPersonalKey(row: components["schemas"]["ApikeyApiKey"]): PersonalApiKey {
  if (!row || typeof row.id !== "string" || typeof row.name !== "string" || typeof row.keyPrefix !== "string") throw new Error("keys.invalid-response");
  return { id: row.id, name: row.name, keyPrefix: row.keyPrefix, createdAt: row.createdAt, expiresAt: row.expiresAt, lastUsedAt: row.lastUsedAt };
}

export function createHttpUsers(baseUrl: string, client: CsrfClient): UsersApi {
  async function request<T>(path: string, method = "GET", body?: object): Promise<T> {
    const response = await client.fetch(`${baseUrl}${path}`, { method, credentials: "same-origin", headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) });
    const payload = await response.json() as { success?: boolean; data?: T; code?: string };
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: payload.code ?? "http.error", status: response.status });
    if (payload.success !== true) throw new Error("users.invalid-response");
    return payload.data as T;
  }
  const userPath = (id: string) => `/users/${encodeURIComponent(id)}`;
  return {
    async listUsers() {
      const rows: ManagedUser[] = [];
      for (;;) {
        // The paginated envelope has totals outside data.
        const response = await client.fetch(`${baseUrl}/users?start=${rows.length}&limit=100`, { credentials: "same-origin", headers: { Accept: "application/json" } });
        const payload = await response.json() as components["schemas"]["BasePaginatedUserUser"];
        if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status });
        if (!payload.success || !Array.isArray(payload.data) || !payload.pagination) throw new Error("users.invalid-page");
        rows.push(...payload.data.map(mapManagedUser));
        if (rows.length >= payload.pagination.totalItems) return rows;
        if (!payload.data.length) throw new Error("users.short-page");
      }
    },
    createUser: async (input) => mapManagedUser(await request("/users", "POST", input)),
    updateUser: async (id, input) => mapManagedUser(await request(userPath(id), "PUT", input)),
    disableUser: async (id) => { await request(`${userPath(id)}/disable`, "POST"); },
    enableUser: async (id) => { await request(`${userPath(id)}/enable`, "POST"); },
    resetUserPassword: async (id, password) => { const input: components["schemas"]["ResetPasswordInputBody"] = { password }; await request(`${userPath(id)}/reset-password`, "POST", input); },
    deleteUser: async (id) => { await request(userPath(id), "DELETE"); },
    setUserAdministrator: async (id, isAdmin) => { const input: components["schemas"]["SetAdminInputBody"] = { isAdmin }; await request(`${userPath(id)}/administrator`, "PUT", input); },
    setUserAppAssignments: async (id, assignments) => { const input: components["schemas"]["SetAssignmentsInputBody"] = { assignments }; await request(`${userPath(id)}/app-assignments`, "PUT", input); },
    listMyApiKeys: async () => (await request<components["schemas"]["ApikeyApiKey"][] | null>("/auth/me/api-keys") ?? []).map(mapPersonalKey),
    createMyApiKey: async (input) => {
      const result = await request<components["schemas"]["ApikeyApiKeyCreatedDto"]>("/auth/me/api-keys", "POST", input);
      if (typeof result.key !== "string" || !result.key) throw new Error("keys.invalid-secret");
      return { ...mapPersonalKey(result), key: result.key };
    },
    deleteMyApiKey: async (id) => { await request(`/auth/me/api-keys/${encodeURIComponent(id)}`, "DELETE"); },
  };
}
