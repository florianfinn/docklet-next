// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original identity-owned inventory scope retention and automatic read lifetimes. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags. M24 S6: observed recovery and update UI follow-ups.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { DockletAdapter } from "../adapter";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import type { Host, ScheduledJob } from "@/domain/model";
import { isMaintenanceError } from "../maintenance-retry";
import type { CsrfClient } from "./csrf-client";
import { nextOccurrence } from "@/domain/schedule";
import { createUpdateSettings } from "./update-settings";
import { createStackUpdates } from "./stack-updates";
import { READ_TAG_HEADER, type ReadTag } from "./host-read-policy";

export function createScheduleReader(baseUrl: string, client: CsrfClient, hosts: () => Promise<Host[]>, capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>, generation: () => number, overview: DockletAdapter["getBackupOverview"], settings: () => ReturnType<NonNullable<DockletAdapter["getUpdateSettings"]>>): DockletAdapter["listSchedule"] {
  let retained = new Map<string, Awaited<ReturnType<DockletAdapter["listSchedule"]>>>();
  let owner = generation();
  return async (hostIds, tag: ReadTag = "background") => {
    const version = generation();
    if (version !== owner) { retained = new Map(); owner = version; }
    const check = () => { if (version !== generation()) throw new CancelledError(); };
    // A host or backup change fences an overview read in flight; within the same identity the schedule reads it once
    // more instead of failing (the schedule is not refreshed by those changes itself).
    const fenced = <T,>(read: () => Promise<T>) => read().catch((error: unknown) => {
      if (!isCancelledError(error) || version !== generation()) throw error;
      return read();
    });
    const catalogue = await hosts(); check();
    const selected = hostIds ?? catalogue.map((h) => h.id);
    let settingsRead: ReturnType<typeof settings> | undefined;
    const readSettings = () => settingsRead ??= settings();
    // Only this automatic fan-out is tagged; deliberate policy dialogs use their normal client.
    const policies = createStackUpdates({ baseUrl, capability, generation, changed() {}, client: { ...client, fetch: (input, init) => {
      const headers = new Headers(init?.headers);
      const host = /\/environments\/([^/]+)\//.exec(String(input))?.[1];
      if (retained.has(decodeURIComponent(host ?? "")) || ["online", "standby"].includes(catalogue.find((h) => h.id === host)?.status ?? "")) headers.set(READ_TAG_HEADER, tag);
      return client.fetch(input, { ...init, headers });
    } } });
    const jobs = createUpdateSettings({ baseUrl, client, capability, generation });
    const batches = await Promise.all(selected.map(async (host) => {
      const previous = retained.get(host) ?? [];
      const [backup, updates, updateCheck] = await Promise.all([
        fenced(() => overview(host, retained.has(host) || ["online", "standby"].includes(catalogue.find((h) => h.id === host)?.status ?? "") ? tag : undefined)).then((o) => o.volumes.filter((v) => v.backup?.enabled && v.backup.nextRunAt).map((v) => ({ id: `${host}~backup-${v.name}`, hostId: host, kind: "backup" as const, title: v.name, volumeId: v.id, cadence: v.backup!.schedule.mode, nextRunAt: v.backup!.nextRunAt, enabled: true })), (error: unknown) => {
          // Maintenance fails the whole read, so the query keeps its cached rows while it retries (S2 B19).
          if (isCancelledError(error) || isMaintenanceError(error)) throw error;
          return previous.filter((r) => r.kind === "backup").map((r) => ({ ...r, unavailable: true }));
        }),
        (async () => {
          try {
            const config = await policies.listUpdatePolicies(host);
            const { timeZone } = await readSettings();
            return config.filter((p) => p.mode === "scheduled" && p.window).map((p) => {
              const window = p.window!;
              const schedule = window.kind === "daily" ? { mode: "daily" as const, time: window.time } : { mode: "weekly" as const, day: (["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const)[window.day!]!, time: window.time };
              return { id: `${host}~${p.targetKind}-${p.targetId}`, hostId: host, kind: "auto-update" as const, title: p.targetName, cadence: window.kind, nextRunAt: nextOccurrence(schedule, new Date(), timeZone)?.toISOString(), enabled: true, code: p.applyCode };
            });
          } catch (error) {
            if (isCancelledError(error) || isMaintenanceError(error)) throw error;
            const old = previous.filter((r) => r.kind === "auto-update");
            return old.length ? old.map((r) => ({ ...r, unavailable: true })) : [{ id: `${host}~schedule-unavailable`, hostId: host, kind: "update-check" as const, title: "", cadence: "", unavailable: true }];
          }
        })(),
        (host === "0" ? jobs.getUpdateCheckJob(host).then((j) => [{ id: `${host}~update-check`, hostId: host, kind: "update-check" as const, title: "", cadence: j.schedule, nextRunAt: j.nextRun, enabled: j.enabled }], (error: unknown) => {
          if (isCancelledError(error) || isMaintenanceError(error)) throw error;
          return previous.filter((r) => r.kind === "update-check" && r.id !== `${host}~schedule-unavailable`).map((r) => ({ ...r, unavailable: true }));
        }) : Promise.resolve([])),
      ]);
      check(); const rows = [...backup, ...updates, ...updateCheck]; retained.set(host, rows.map((r) => ({ ...r }))); return { backup, updates: [...updates, ...updateCheck] };
    }));
    check(); return [...batches.flatMap<ScheduledJob>((b) => b.backup), ...batches.flatMap<ScheduledJob>((b) => b.updates)];
  };
}
