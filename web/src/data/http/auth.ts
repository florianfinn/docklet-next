// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Connected sign-in and profile HTTP calls.

import type { AuthApi, CurrentUser } from "@/data/api";
import type { AuthMode } from "@/domain/settings";

import { AdapterOutcomeError } from "../capability";
import type { CsrfClient } from "./csrf-client";

function user(value: unknown): CurrentUser {
  if (!value || typeof value !== "object") throw new Error("auth.invalid-user");
  const item = value as Record<string, unknown>;
  if (typeof item.id !== "string" || typeof item.username !== "string" || typeof item.requiresPasswordChange !== "boolean") throw new Error("auth.invalid-user");
  return { id: item.id, username: item.username, requiresPasswordChange: item.requiresPasswordChange, locale: typeof item.locale === "string" ? item.locale : null };
}

export function createHttpAuth(baseUrl: string, client: CsrfClient, onSessionChange: () => void = () => {}): AuthApi {
  let currentUserId: string | undefined;
  function sessionChanged(id?: string) { currentUserId = id; onSessionChange(); }
  async function request(path: string, method = "GET", body?: object): Promise<unknown> {
    let response: Response;
    try {
      response = await client.fetch(`${baseUrl}${path}`, {
        method,
        credentials: "same-origin",
        headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
    } catch {
      throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    }
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status });
    const payload = await response.json() as { success?: boolean; data?: unknown; mode?: unknown };
    if (payload.success !== true) throw new Error("auth.invalid-response");
    return payload.data;
  }

  return {
    async getAuthMode() {
      const payload = await request("/auth/mode") as { mode?: unknown };
      if (payload.mode !== "local" && payload.mode !== "proxy" && payload.mode !== "none") throw new Error("auth.invalid-mode");
      return payload.mode as AuthMode;
    },
    async getCurrentUser() {
      const result = user(await request("/auth/me"));
      if (result.id !== currentUserId) sessionChanged(result.id);
      return result;
    },
    async login(username, password) {
      const result = await request("/auth/login", "POST", { username, password }) as { user?: unknown };
      const signedIn = user(result.user);
      sessionChanged(signedIn.id);
      return signedIn;
    },
    async logout() { await request("/auth/logout", "POST"); sessionChanged(); },
    async changePassword(currentPassword, newPassword) { await request("/auth/password", "POST", { currentPassword, newPassword }); sessionChanged(); },
    async updateMyLocale(locale) { return user(await request("/auth/me/profile", "PUT", { locale })); },
  };
}
