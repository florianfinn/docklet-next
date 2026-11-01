// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live volume backup transport with host ownership and asynchronous admission.

import { CancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { BackupError, backupCodeKey } from "@/data/backup-error";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import type { Host } from "@/domain/model";
import type { BackupOverview } from "@/domain/volumes";
import type { CsrfClient } from "./csrf-client";
import { inventoryId, splitInventoryId } from "./inventory-mapping";
import { backupCron, mapBackup, mapBackupOverview } from "./backup-mapping";

type Schema = components["schemas"];
export const BACKUP_OPERATIONS = [
  "setBackupPolicy",
  "runBackup",
  "restoreBackup",
  "restoreBackupFiles",
  "browseBackup",
  "downloadBackup",
  "deleteBackup",
  "uploadRestore",
  "getBackupOverview",
  "listBackupOverviews",
  "getBackupRepository",
  "checkBackupRepository",
  "listBackupTargets",
  "createBackupTarget",
  "updateBackupTarget",
  "deleteBackupTarget",
  "testBackupTarget",
  "getBackupTargetReferences",
  "getBackupRecoveryKey",
  "attachLocalBackupRecoveryKey",
] as const;
type Operation = (typeof BACKUP_OPERATIONS)[number];
const ACCESS_CODES = new Set(["agent.unavailable", "agent.invalid-token", "agent.update-required"]);
interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number;
  hosts: () => Promise<Host[]>;
  changed: () => void;
  accepted: (host: string, activityId: string) => void;
}

export function createBackups(options: Options) {
  const { baseUrl, client } = options;
  const versions = new Map<string, number>();
  const retained = new Map<string, BackupOverview>();
  let retainedOwner = options.generation();
  const path = (host: string) => `/environments/${encodeURIComponent(host)}/volumes`;
  function begin(host: string) {
    if (!versions.has(host)) versions.set(host, 0);
    const identity = options.generation(),
      version = versions.get(host) ?? 0;
    return () => {
      if (identity !== options.generation() || version !== (versions.get(host) ?? 0)) throw new CancelledError();
    };
  }
  async function require(operation: string, ref: ResourceRef) {
    const descriptor = await options.capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, ref, descriptor.disabledReasonCode);
  }
  function problem(body: unknown, status: number, write = false): AdapterOutcomeError {
    const value = body && typeof body === "object" ? (body as Schema["BackupProblem"]) : undefined;
    // Access refusals keep their code; an unreachable agent leaves a write's outcome open.
    if (value && ACCESS_CODES.has(value.code) && !(write && (value.code as string) === "agent.unavailable"))
      return new AdapterOutcomeError({ kind: "error", code: value.code, status });
    if (value && backupCodeKey(value.code))
      return new BackupError(
        {
          code: value.code,
          activityId: value.activityId,
          reason: value.reason,
          hosts: value.hosts,
          volumes: value.volumes,
        },
        status,
      );
    return new AdapterOutcomeError({
      kind: "error",
      code: status === 502 || status === 504 ? "volume.outcome-unknown" : "http.error",
      status,
    });
  }
  async function response(host: string, route: string, init: RequestInit = {}) {
    const check = begin(host);
    let result: Response;
    try {
      result = await client.fetch(`${baseUrl}${route}`, {
        credentials: "same-origin",
        cache: "no-store",
        ...init,
        headers: { Accept: "application/json", ...init.headers },
      });
    } catch {
      check();
      throw new AdapterOutcomeError({
        kind: "error",
        code: init.method && init.method !== "GET" ? "volume.outcome-unknown" : "http.error",
      });
    }
    check();
    if (!result.ok) {
      const body: unknown = await result.json().catch(() => null);
      check();
      throw problem(body, result.status, !!init.method && init.method !== "GET");
    }
    return { result, check };
  }
  async function request<T>(host: string, route: string, method = "GET", value?: object): Promise<T> {
    const { result, check } = await response(host, route, {
      method,
      ...(value ? { headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) } : {}),
    });
    let body: { success?: boolean; data: T };
    try {
      body = await result.json();
    } catch {
      check();
      throw new AdapterOutcomeError({
        kind: "error",
        code: method === "GET" ? "http.error" : "volume.outcome-unknown",
      });
    }
    check();
    if (!body || body.success !== true || !("data" in body))
      throw new AdapterOutcomeError({
        kind: "error",
        code: method === "GET" ? "http.error" : "volume.outcome-unknown",
      });
    return body.data;
  }
  async function overview(host: string) {
    return mapBackupOverview(
      await request<Schema["VolumeBackupOverview"]>(host, `${path(host)}/backups/overview`),
      host,
    );
  }
  function context(id: string) {
    const [host, name] = splitInventoryId(id);
    if (!name || name.startsWith("bind:")) throw unavailable("runBackup", { kind: "volume", id });
    return { host, name, route: `${path(host)}/${encodeURIComponent(name)}` };
  }
  async function admit(host: string, route: string, value: object, volume?: string): Promise<string> {
    const check = begin(host);
    let id: string;
    try {
      id = (await request<{ activityId: string }>(host, route, "POST", value)).activityId;
      if (typeof id !== "string" || !id)
        throw new AdapterOutcomeError({ kind: "error", code: "volume.outcome-unknown" });
    } catch (error) {
      check();
      if (error instanceof BackupError && error.problem.activityId) options.accepted(host, error.problem.activityId);
      if (
        error instanceof AdapterOutcomeError &&
        error.outcome.kind === "error" &&
        error.outcome.code === "volume.outcome-unknown" &&
        volume
      ) {
        const observed = await overview(host).catch(() => undefined);
        check();
        const running = observed?.volumes.find((v) => v.name === volume)?.running;
        if (running) {
          options.accepted(host, running.activityId);
          options.changed();
          return running.activityId;
        }
      }
      throw error;
    }
    check();
    options.accepted(host, id);
    options.changed();
    return id;
  }
  const api: Pick<DockletAdapter, Operation> = {
    async getBackupOverview(host) {
      await require("getBackupOverview", { kind: "host", id: host });
      return overview(host);
    },
    async listBackupOverviews(onPartial) {
      const check = begin("");
      await require("listBackupOverviews", { kind: "hub", id: "0" });
      const hosts = await options.hosts();
      check();
      if (retainedOwner !== options.generation()) {
        retained.clear();
        retainedOwner = options.generation();
      }
      const batches = new Map([...retained].filter(([id]) => hosts.some((h) => h.id === id)));
      await Promise.all(
        hosts.map(async (host) => {
          const hostCheck = begin(host.id);
          try {
            const row = await api.getBackupOverview(host.id);
            check();
            hostCheck();
            retained.set(host.id, row);
            batches.set(host.id, row);
          } catch (error) {
            check();
            hostCheck();
            if (error instanceof CancelledError) throw error;
            const old = retained.get(host.id);
            batches.set(host.id, {
              hostId: host.id,
              timeZone: old?.timeZone ?? "",
              authority: "",
              setVersion: 0,
              targets: old?.targets ?? [],
              volumes: (old?.volumes ?? []).map((v) => ({ ...v, backupUnavailable: true })),
              unavailable: true,
            });
          }
          onPartial?.([...batches.values()]);
        }),
      );
      check();
      return [...batches.values()];
    },
    async setBackupPolicy(id, policy) {
      const { host, route } = context(id);
      const check = begin(host);
      await require("setBackupPolicy", { kind: "volume", id });
      const current = await overview(host);
      check();
      const body: Schema["VolumeUpdateBackupPolicies"] = {
        authority: current.authority,
        setVersion: current.setVersion,
        policies: policy
          ? [
              {
                ...(policy.id ? { id: policy.id } : {}),
                enabled: policy.schedule.mode !== "manual",
                localEnabled: true,
                retentionCount: policy.retentionCount,
                stopContainers: policy.stopContainers,
                targetId: policy.targetId,
                schedule: backupCron(policy.schedule),
              },
            ]
          : [],
      };
      try {
        await request(host, `${route}/backup-policy`, "PUT", body);
      } finally {
        options.changed();
      }
    },
    async runBackup(id, value = {}) {
      const { host, route, name } = context(id);
      await require("runBackup", { kind: "volume", id });
      return admit(host, `${route}/backups`, value, name);
    },
    async restoreBackup(id, runId, mode, newVolumeName) {
      const { host, route, name } = context(id);
      await require("restoreBackup", { kind: "volume", id });
      return admit(
        host,
        `${route}/backups/${encodeURIComponent(runId)}/restore`,
        { mode, ...(newVolumeName ? { newVolumeName } : {}) },
        name,
      );
    },
    async restoreBackupFiles(id, runId, selection) {
      const { host, route, name } = context(id);
      await require("restoreBackupFiles", { kind: "volume", id });
      return admit(host, `${route}/backups/${encodeURIComponent(runId)}/restore-files`, selection, name);
    },
    async browseBackup(id, runId, folder = "/", start = 0) {
      const { host } = context(id);
      await require("browseBackup", { kind: "volume", id });
      // The UI roots folders at "/"; the producer takes paths relative to the snapshot root.
      const query = new URLSearchParams({ path: folder.replace(/^\/+/, ""), start: String(start), limit: "100" });
      const { result, check } = await response(
        host,
        `${path(host)}/backups/${encodeURIComponent(runId)}/files/browse?${query}`,
      );
      const body = (await result.json()) as Schema["BasePaginatedBackupBackupFileEntry"];
      check();
      if (!body.success || !body.pagination) throw problem(null, 200);
      return body;
    },
    async downloadBackup(id, runId) {
      const { host, name } = context(id);
      await require("downloadBackup", { kind: "volume", id });
      const route = `${path(host)}/backups/${encodeURIComponent(runId)}/download`;
      // Direct and hub-local archives stream to disk through a browser download; Edge keeps the
      // bounded buffered read with its early size refusal and typed messages.
      if ((await options.hosts()).find((h) => h.id === host)?.connection !== "edge") {
        const link = document.createElement("a");
        link.href = `${baseUrl}${route}`;
        link.download = `${name}-${runId}.tar.gz`;
        document.body.append(link);
        link.click();
        link.remove();
        return undefined;
      }
      const { result, check } = await response(host, route);
      const blob = await result.blob();
      check();
      return blob;
    },
    async deleteBackup(id, runId) {
      const { host } = context(id);
      await require("deleteBackup", { kind: "volume", id });
      try {
        await request(host, `${path(host)}/backups/${encodeURIComponent(runId)}`, "DELETE");
      } finally {
        options.changed();
      }
    },
    async uploadRestore(id, file, mode = "replace", newVolumeName) {
      const { host, route, name } = context(id);
      const check = begin(host);
      await require("uploadRestore", { kind: "volume", id });
      const uploadRoute = `/environments/${encodeURIComponent(host)}/uploads/volume-backup`;
      const session = await request<Schema["UploadSession"]>(host, uploadRoute, "POST", {
        filename: file.name,
        size: file.size,
      });
      check();
      if (session.chunkSize <= 0 || session.totalChunks < 0) throw problem(null, 200);
      try {
        for (let index = 0; index < session.totalChunks; index++) {
          check();
          const chunk = await response(host, `${uploadRoute}/${encodeURIComponent(session.id)}/chunks/${index}`, {
            method: "PUT",
            headers: { "Content-Type": "application/octet-stream" },
            body: file.slice(index * session.chunkSize, (index + 1) * session.chunkSize),
          });
          await chunk.result.arrayBuffer();
          chunk.check();
        }
        check();
        return await admit(
          host,
          `${route}/backups/upload`,
          { uploadId: session.id, mode, ...(newVolumeName ? { newVolumeName } : {}) },
          name,
        );
      } catch (error) {
        if (!(error instanceof CancelledError))
          void request(host, `${uploadRoute}/${encodeURIComponent(session.id)}`, "DELETE").catch(() => {});
        throw error;
      }
    },
    async getBackupRepository(host) {
      await require("getBackupRepository", { kind: "host", id: host });
      return request(host, `${path(host)}/backups/repository`);
    },
    async checkBackupRepository(host) {
      await require("checkBackupRepository", { kind: "host", id: host });
      return admit(host, `${path(host)}/backups/repository/check`, {});
    },
    async listBackupTargets() {
      await require("listBackupTargets", { kind: "hub", id: "0" });
      return (await request<Schema["BackupTarget"][] | null>("", "/backup-targets")) ?? [];
    },
    async createBackupTarget(input) {
      await require("createBackupTarget", { kind: "hub", id: "0" });
      try {
        return await request("", "/backup-targets", "POST", input);
      } finally {
        options.changed();
      }
    },
    async updateBackupTarget(id, input) {
      await require("updateBackupTarget", { kind: "hub", id: "0" });
      try {
        return await request("", `/backup-targets/${encodeURIComponent(id)}`, "PUT", input);
      } finally {
        options.changed();
      }
    },
    async deleteBackupTarget(id) {
      await require("deleteBackupTarget", { kind: "hub", id: "0" });
      try {
        await request("", `/backup-targets/${encodeURIComponent(id)}`, "DELETE");
      } finally {
        options.changed();
      }
    },
    async testBackupTarget(id, host) {
      await require("testBackupTarget", id === "local" ? { kind: "host", id: host } : { kind: "hub", id: "0" });
      return admit(
        host,
        id === "local" ? `${path(host)}/backups/targets/local/test` : `/backup-targets/${encodeURIComponent(id)}/test`,
        { hostId: host },
      );
    },
    async getBackupTargetReferences(id, host) {
      await require("getBackupTargetReferences", { kind: "host", id: host });
      return request(host, `${path(host)}/backups/targets/${encodeURIComponent(id)}/references`);
    },
    async getBackupRecoveryKey(id, host) {
      await require("getBackupRecoveryKey", id === "local" && host
        ? { kind: "host", id: host }
        : { kind: "hub", id: "0" });
      return (
        await request<Schema["BackupTargetRecoveryKey"]>(
          host ?? "",
          id === "local" && host
            ? `${path(host)}/backups/targets/local/recovery-key`
            : `/backup-targets/${encodeURIComponent(id)}/recovery-key`,
        )
      ).recoveryKey;
    },
    async attachLocalBackupRecoveryKey(host, recoveryKey) {
      await require("attachLocalBackupRecoveryKey", { kind: "host", id: host });
      await request(host, `${path(host)}/backups/targets/local/recovery-key`, "PUT", { recoveryKey });
    },
  };
  return {
    api,
    guard: begin,
    invalidate(hosts?: string[]) {
      for (const host of hosts ?? versions.keys()) {
        versions.set(host, (versions.get(host) ?? 0) + 1);
        retained.delete(host);
      }
    },
    async history(id: string) {
      const { host, route } = context(id);
      const check = begin(host);
      const out = [];
      for (let start = 0; ; ) {
        const { result } = await response(host, `${route}/backups?start=${start}&limit=100&order=desc`);
        const body = (await result.json()) as Schema["VolumeBackupPaginatedResponse"];
        check();
        if (!body.success || !body.pagination) throw problem(null, 200);
        const rows = body.data ?? [];
        out.push(...rows.map((row) => mapBackup(row, host)));
        start += rows.length;
        if (start >= body.pagination.totalItems) return out;
        if (!rows.length) throw problem(null, 200);
      }
    },
    async decorate(host: string, volumes: Awaited<ReturnType<DockletAdapter["listVolumes"]>>) {
      const ov = await api.getBackupOverview(host).catch((error) => {
        if (error instanceof CancelledError) throw error;
        return undefined;
      });
      const byId = new Map(ov?.volumes.map((v) => [v.id, v]));
      return volumes.map((v) =>
        v.kind === "bind"
          ? v
          : {
              ...v,
              backupUnavailable: !ov,
              ...byId.get(v.id),
              sizeBytes: v.sizeBytes,
              consumers: v.consumers,
              labels: v.labels,
              source: v.source,
              createdAt: v.createdAt,
              sizeUnavailable: v.sizeUnavailable,
            },
      );
    },
    inventoryId,
  };
}
