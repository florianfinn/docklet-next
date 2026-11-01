// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original generated-DTO-shaped backup transport fixtures.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createBackups } from "./backups";
import type { Host } from "@/domain/model";
import { HOSTS } from "@/data/demo/fixtures/hosts";

type Schema = components["schemas"];
export const entry: Schema["VolumeBackupEntry"] = { id: "snapshot", volumeName: "data", destination: "local", createdAt: "2026-10-28T02:00:00Z", status: "partial", trigger: "safety", size: 42, localSnapshotId: "snapshot-hash", stopContainers: true, format: "rustic", type: "volume", errorCode: "volume.backup.restart-failed" };
export const policy: Schema["VolumeBackupPolicy"] = { id: "policy", volumeName: "data", targetId: "local", enabled: true, localEnabled: true, retentionCount: 7, schedule: "0 0 2 * * *", stopContainers: false, nextRunAt: "2026-11-01T01:00:00Z" };
export const overview: Schema["VolumeBackupOverview"] = { timeZone: "Europe/Berlin", backupTargetAuthority: "authority", backupTargetSetVersion: 7, repository: { mode: "volume", available: true }, targets: [{ id: "local", name: "Local", kind: "local" }], volumes: [{ name: "data", volumeExists: true, count: 1, policy, lastBackup: entry }] };
export const target: Schema["BackupTarget"] = { id: "target", name: "Archive", kind: "smb", server: "nas.example.com", share: "backups", path: "docklet", hasSecret: true, createdAt: "2026-10-28T02:00:00Z", configVersion: 7, checks: [{ hostId: "first", activityId: "test", at: "2026-10-28T02:00:00Z", ok: true, configVersion: 6, setVersion: 6, earlierSettings: true }] };
export const repository: Schema["VolumeBackupRepository"] = { mode: "volume", name: "arcane-backups", engineImage: "ghcr.io/rustic-rs/rustic:v0.11.2", lastCheck: { at: "2026-10-28T02:00:00Z", ok: true, initialized: true } };
export const ok = (data: unknown, status = 200) => Response.json({ success: true, data }, { status });
export function backupFixture() {
  let generation = 0;
  const state = { overview: structuredClone(overview), requests: [] as { path: string; method: string; body?: unknown }[], key: "SYNTHETIC-RECOVERY-KEY", fail: "", code: "volume.backup.busy", reason: undefined as string | undefined, calls: 0 };
  let intercept: ((url: URL, init?: RequestInit) => Promise<Response | undefined>) | undefined;
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), method = init?.method ?? "GET";
    let body: unknown; if (typeof init?.body === "string") body = JSON.parse(init.body);
    state.requests.push({ path: url.pathname, method, body });
    const replacement = await intercept?.(url, init); if (replacement) return replacement;
    if (state.fail && url.pathname.endsWith(state.fail)) return Response.json({ code: state.code, detail: "PRIVATE SERVER TEXT", title: "PRIVATE SERVER TITLE", status: 409, reason: state.reason, activityId: state.code === "volume.backup.busy" ? "held" : undefined, hosts: ["second"], volumes: ["data"] }, { status: 409 });
    if (url.pathname.endsWith("/overview")) return ok(state.overview);
    if (url.pathname.endsWith("/backup-policy")) return ok({ policies: [policy], timeZone: "Europe/Berlin" });
    if (url.pathname.endsWith("/recovery-key")) return ok({ recoveryKey: state.key });
    if (url.pathname.endsWith("/repository")) return ok(repository);
    if (url.pathname === "/api/backup-targets") return ok(method === "GET" ? [target] : target);
    // The producer lists paths relative to the snapshot root and refuses absolute ones.
    if (url.pathname.endsWith("/files/browse") && url.searchParams.get("path")?.startsWith("/")) return Response.json({ status: 400, detail: "PRIVATE SERVER TEXT" }, { status: 400 });
    if (url.pathname.endsWith("/files/browse")) return Response.json({ success: true, data: [{ name: "folder", path: "folder", isDirectory: true }], pagination: { totalItems: 150, currentPage: 1, itemsPerPage: 100, totalPages: 2 } });
    if (url.pathname.endsWith("/backups") && method === "GET") return Response.json({ success: true, data: [entry], pagination: { totalItems: 1, currentPage: 1, itemsPerPage: 100, totalPages: 1 } });
    if (url.pathname.endsWith("/volume-backup") && method === "POST") return ok({ id: "upload", chunkSize: 2, totalChunks: 2, complete: false });
    if (url.pathname.includes("/chunks/")) return ok({ complete: true });
    if (url.pathname.endsWith("/download")) return new Response("archive");
    return ok({ activityId: "accepted", restoreId: "restore" }, 202);
  });
  const capability = vi.fn(async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: true }));
  const accepted = vi.fn(), changed = vi.fn();
  const hosts = ["first", "second"].map((id) => ({ ...HOSTS[0]!, id, name: id, status: "online", connection: id === "second" ? "edge" : "direct" }) as Host);
  const service = createBackups({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "synthetic" }, generation: () => generation, capability, hosts: async () => hosts, accepted, changed });
  return { ...service, state, fetcher, accepted, changed, capability, hosts, rotate: () => ++generation, intercept: (fn: typeof intercept) => { intercept = fn; } };
}
