// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original HTTP mapping for the Hub backup UI in issue #32.

import type { components, operations } from "@/api/generated/schema";
import type { HubBackupApi, HubBackupAccess, HubBackupOperation, HubBackupPolicy, HubBackupRun } from "@/domain/hub-backup";

import { AdapterOutcomeError } from "../capability";

type User = components["schemas"]["UserUser"];
type Activity = operations["get-activity"]["responses"][200]["content"]["application/json"];

function responseCode(body: unknown): string {
  if (!body || typeof body !== "object") return "http.error";
  const value = body as { code?: unknown; detail?: unknown; title?: unknown };
  for (const entry of [value.code, value.detail, value.title]) {
    if (typeof entry === "string" && /^[a-z][a-z0-9_.-]+$/.test(entry)) return entry;
  }
  return "http.error";
}

function error(body: unknown, status: number): AdapterOutcomeError {
  return new AdapterOutcomeError({ kind: "error", code: responseCode(body), status });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function createHttpHubBackups(baseUrl: string, fetcher: typeof fetch, uploadFactory: () => XMLHttpRequest = () => new XMLHttpRequest()): HubBackupApi {
  async function request(route: string, init?: RequestInit): Promise<unknown> {
    let response: Response;
    try {
      response = await fetcher(`${baseUrl}${route}`, {
        credentials: "same-origin", headers: { Accept: "application/json", ...init?.headers }, ...init,
      });
    } catch { throw error(null, 0); }
    let body: unknown;
    try { body = await response.json(); } catch { throw error(null, response.status); }
    if (!response.ok) throw error(body, response.status);
    return body;
  }

  function data(body: unknown): unknown {
    if (!isRecord(body) || body.success !== true || !("data" in body)) throw error({ code: "backup.invalid_response" }, 200);
    return body.data;
  }

  async function list(): Promise<HubBackupRun[]> {
    const result: HubBackupRun[] = [];
    let start = 0;
    for (;;) {
      const body = await request(`/backups?start=${start}&limit=100&sort=createdAt&order=desc`);
      const batch = data(body);
      if (!Array.isArray(batch) || !isRecord(body) || !isRecord(body.pagination) || typeof body.pagination.totalItems !== "number") {
        throw error({ code: "backup.invalid_response" }, 200);
      }
      result.push(...batch as HubBackupRun[]);
      start += batch.length;
      if (start >= body.pagination.totalItems) return result;
      if (batch.length === 0) throw error({ code: "backup.invalid_response" }, 200);
    }
  }

  return {
    async getHubBackupAccess(): Promise<HubBackupAccess> {
      let body: unknown;
      try { body = await request("/auth/me"); }
      catch (cause) {
        if (cause instanceof AdapterOutcomeError && cause.outcome.kind === "error" && (cause.outcome.status === 401 || cause.outcome.status === 403)) return { read: false, manage: false, restore: false };
        throw cause;
      }
      const user = data(body) as User;
      if (!isRecord(user) || typeof user.isGlobalAdmin !== "boolean") throw error({ code: "backup.invalid_response" }, 200);
      const global = user.permissionsByEnv?.global ?? [];
      const permitted = (name: string) => user.isGlobalAdmin && Array.isArray(global) && (global.includes(name) || global.includes("*"));
      return { read: permitted("system-backups:read"), manage: permitted("system-backups:manage"), restore: permitted("system-backups:restore") };
    },
    listHubBackups: list,
    async getHubBackupPolicies() {
      const body = await request("/backups/policies");
      if (!isRecord(body) || !Array.isArray(body.policies)) throw error({ code: "backup.invalid_response" }, 200);
      return body.policies as HubBackupPolicy[];
    },
    async saveHubBackupPolicies(policies) {
      const body = await request("/backups/policies", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ policies: policies.map(({ id, enabled, retentionCount, schedule }) => ({ id, enabled, retentionCount, schedule })) }) });
      if (!isRecord(body) || !Array.isArray(body.policies)) throw error({ code: "backup.invalid_response" }, 200);
      return body.policies as HubBackupPolicy[];
    },
    async getHubBackupStorage() {
      const body = await request("/backups/storage");
      if (!isRecord(body) || typeof body.available !== "boolean") throw error({ code: "backup.invalid_response" }, 200);
      return body as components["schemas"]["BackupSystemBackupStorage"];
    },
    async getHubBackupMaintenance() {
      let body: unknown;
      try { body = await request("/backups/maintenance"); }
      catch (cause) {
        // A restore revokes the requesting session; normal admission then resumes.
        if (cause instanceof AdapterOutcomeError && cause.outcome.kind === "error" && (cause.outcome.status === 401 || cause.outcome.status === 403)) return { active: false };
        throw cause;
      }
      if (!isRecord(body) || typeof body.active !== "boolean") throw error({ code: "backup.invalid_response" }, 200);
      return body as components["schemas"]["BackupSystemBackupMaintenance"];
    },
    async getHubBackupActivityStatus(activityId) {
      const body = data(await request(`/environments/0/activities/${encodeURIComponent(activityId)}`)) as Activity["data"];
      if (!body?.activity || typeof body.activity.status !== "string") throw error({ code: "backup.invalid_response" }, 200);
      return body.activity.status;
    },
    async createHubBackup() {
      const body = await request("/backups", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
      if (!isRecord(body) || typeof body.id !== "string") throw error({ code: "backup.invalid_response" }, 200);
      return body as HubBackupRun;
    },
    uploadHubBackup(file, onProgress) {
      return new Promise<HubBackupOperation>((resolve, reject) => {
        const xhr = uploadFactory();
        xhr.open("POST", `${baseUrl}/backups/upload`);
        xhr.withCredentials = true;
        xhr.setRequestHeader("Content-Type", "application/octet-stream");
        xhr.setRequestHeader("Accept", "application/json");
        xhr.upload.onprogress = (event) => { if (event.lengthComputable) onProgress(Math.round(event.loaded / event.total * 100)); };
        xhr.onerror = () => reject(error(null, 0));
        xhr.onload = () => {
          let body: unknown;
          try { body = JSON.parse(xhr.responseText); } catch { reject(error(null, xhr.status)); return; }
          if (xhr.status < 200 || xhr.status >= 300) { reject(error(body, xhr.status)); return; }
          if (!isRecord(body) || typeof body.backupId !== "string") { reject(error({ code: "backup.invalid_response" }, xhr.status)); return; }
          onProgress(100);
          resolve(body as HubBackupOperation);
        };
        xhr.send(file);
      });
    },
    async restoreHubBackup(id) {
      const body = await request(`/backups/${encodeURIComponent(id)}/restore`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ confirmBackupId: id }) });
      if (!isRecord(body) || typeof body.activityId !== "string") throw error({ code: "backup.invalid_response" }, 200);
      return body as HubBackupOperation;
    },
    async downloadHubBackup(id) {
      const link = document.createElement("a");
      link.href = `${baseUrl}/backups/${encodeURIComponent(id)}/download`;
      link.download = "";
      document.body.append(link);
      link.click();
      link.remove();
    },
  };
}
