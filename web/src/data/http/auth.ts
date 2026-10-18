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

export type SessionChange = "established" | "renewed" | "confirmed" | "lost";

// Replacement clears observations; renewal keeps same-user history; loss never reconnects.
export function createHttpAuth(baseUrl: string, client: CsrfClient, onSessionChange: (change: SessionChange) => void = () => {}, sessionGeneration: () => number = () => 0): AuthApi {
  let currentUserId: string | undefined;
  let sessionVersion = 0;
  let identityEpoch = 0;
  let renewedStamp: number | undefined;
  let renewedFrom: number | undefined;
  function sessionChanged(change: SessionChange, id?: string, from?: number) {
    renewedFrom = change === "renewed" && from === renewedStamp ? renewedFrom : from;
    ++sessionVersion;
    if (change !== "renewed") ++identityEpoch;
    currentUserId = id;
    onSessionChange(change);
    renewedStamp = change === "renewed" && id ? sessionVersion + sessionGeneration() : undefined;
  }
  function assertCurrent(version: number) {
    if (version !== sessionVersion + sessionGeneration()) throw new CancelledError();
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

  async function getCurrentUser(rereadRenewal = true): Promise<CurrentUser> {
    const version = sessionVersion + sessionGeneration();
    const epoch = identityEpoch;
    const identity = currentUserId;
    try {
      const result = user(await request("/auth/me"));
      assertCurrent(version);
      if (result.id !== currentUserId) sessionChanged("established", result.id);
      else onSessionChange("confirmed");
      return result;
    } catch (error) {
      if (version !== sessionVersion + sessionGeneration()) {
        // Revalidate once after known same-user renewal; never accept the old response.
        if (rereadRenewal && identity && identity === currentUserId && epoch === identityEpoch && renewedFrom !== undefined && version >= renewedFrom && renewedStamp === sessionVersion + sessionGeneration()) return getCurrentUser(false);
        throw new CancelledError();
      }
      if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 401) sessionChanged("lost");
      throw error;
    }
  }

  return {
    async getAuthMode() {
      const payload = await request("/auth/mode") as { mode?: unknown };
      if (payload.mode !== "local" && payload.mode !== "proxy" && payload.mode !== "none") throw new Error("auth.invalid-mode");
      return payload.mode as AuthMode;
    },
    getCurrentUser,
    async login(username, password) {
      const from = sessionVersion + sessionGeneration();
      const version = ++sessionVersion + sessionGeneration();
      const result = await request("/auth/login", "POST", { username, password }) as { user?: unknown };
      const signedIn = user(result.user);
      assertCurrent(version);
      sessionChanged(signedIn.id === currentUserId ? "renewed" : "established", signedIn.id, from);
      return signedIn;
    },
    async logout() {
      const version = ++sessionVersion + sessionGeneration();
      await request("/auth/logout", "POST");
      assertCurrent(version);
      sessionChanged("lost");
    },
    async changePassword(currentPassword, newPassword) {
      const from = sessionVersion + sessionGeneration();
      const version = ++sessionVersion + sessionGeneration();
      await request("/auth/password", "POST", { currentPassword, newPassword });
      assertCurrent(version);
      sessionChanged("renewed", currentUserId, from);
    },
    async updateMyLocale(locale) { return user(await request("/auth/me/profile", "PUT", { locale })); },
  };
}
