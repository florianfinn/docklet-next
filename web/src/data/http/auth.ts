// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Connected sign-in and profile HTTP calls.

import { CancelledError } from "@tanstack/react-query";

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

export type SessionChange = "established" | "confirmed" | "lost";

// Established sessions renew consumers; confirmations resume suspended consumers; loss never reconnects.
export function createHttpAuth(baseUrl: string, client: CsrfClient, onSessionChange: (change: SessionChange) => void = () => {}, sessionGeneration: () => number = () => 0): AuthApi {
  let currentUserId: string | undefined;
  let sessionVersion = 0;
  function sessionChanged(change: SessionChange, id?: string) {
    ++sessionVersion;
    currentUserId = id;
    onSessionChange(change);
  }
  function assertCurrent(version: number) {
    if (version !== sessionVersion + sessionGeneration()) throw new CancelledError({ silent: true });
  }
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
      const version = sessionVersion + sessionGeneration();
      try {
        const result = user(await request("/auth/me"));
        assertCurrent(version);
        if (result.id !== currentUserId) sessionChanged("established", result.id);
        else onSessionChange("confirmed");
        return result;
      } catch (error) {
        assertCurrent(version);
        if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) sessionChanged("lost");
        throw error;
      }
    },
    async login(username, password) {
      const version = ++sessionVersion + sessionGeneration();
      const result = await request("/auth/login", "POST", { username, password }) as { user?: unknown };
      const signedIn = user(result.user);
      assertCurrent(version);
      sessionChanged("established", signedIn.id);
      return signedIn;
    },
    async logout() {
      const version = ++sessionVersion + sessionGeneration();
      await request("/auth/logout", "POST");
      assertCurrent(version);
      sessionChanged("lost");
    },
    async changePassword(currentPassword, newPassword) {
      const version = ++sessionVersion + sessionGeneration();
      await request("/auth/password", "POST", { currentPassword, newPassword });
      assertCurrent(version);
      sessionChanged("established", currentUserId);
    },
    async updateMyLocale(locale) { return user(await request("/auth/me/profile", "PUT", { locale })); },
  };
}
