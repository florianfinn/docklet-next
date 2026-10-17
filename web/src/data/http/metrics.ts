// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Existing per-host stats streams with retained observations.

import type { Host, HostMetrics, InventoryObservation, MetricsSnapshot } from "@/domain/model";
import { inventoryId, mapContainer } from "./inventory-mapping";
import { createInventoryTransport, inventoryFailure } from "./inventory-transport";

export interface MetricsSocket {
  onmessage: ((event: MessageEvent) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  close(): void;
}
export function mapHostMetrics(value: unknown, hostId: string, previous?: HostMetrics): HostMetrics {
  const r = value as Record<string, unknown> | null;
  const number = (key: string) => {
    const n = r?.[key];
    if (typeof n !== "number" || !Number.isFinite(n) || n < 0) throw new Error("metrics.invalid-sample");
    return n;
  };
  const cpu = number("cpuUsage"), used = number("memoryUsage"), total = number("memoryTotal"), cores = number("cpuCount");
  if (!total || !cores || cpu > 100 || used > total) throw new Error("metrics.invalid-sample");
  const mem = used / total * 100;
  const diskTotal = r?.diskTotal == null ? 0 : number("diskTotal");
  const diskUsed = r?.diskUsage == null ? 0 : number("diskUsage");
  if (diskUsed > diskTotal) throw new Error("metrics.invalid-sample");
  return { hostId, cpu, mem, disk: diskTotal ? diskUsed / diskTotal * 100 : 0, diskUnavailable: !diskTotal, growthUnavailable: true, diskGrowthGbPerDay: 0,
    cpuCores: cores, memTotalGb: total / 1024 ** 3, diskTotalGb: diskTotal / 1024 ** 3,
    cpuHistory: [...previous?.cpuHistory ?? [], cpu].slice(-60), memHistory: [...previous?.memHistory ?? [], mem].slice(-60), inventory: { stale: false, observedAt: new Date().toISOString() } };
}

export function createMetricsReader(baseUrl: string, fetcher: typeof fetch, listHosts: () => Promise<Host[]>, requireCapability: (op: string) => Promise<void>, socketFactory: (url: string) => MetricsSocket, onState: (hostId: string, state: InventoryObservation) => void) {
  const retained: Record<string, HostMetrics> = {};
  const containers: MetricsSnapshot["containers"] = {};
  const listeners = new Set<(snapshot: MetricsSnapshot) => void>();
  const sockets = new Map<string, MetricsSocket>();
  const { list } = createInventoryTransport(baseUrl, fetcher, (hostId, state) => { if (state.stale) { stale(hostId, state.reason); emit(); } });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let generation = 0;
  function emit() {
    const snapshot: MetricsSnapshot = { at: new Date().toISOString(), simulated: false, hosts: { ...retained }, containers: { ...containers } };
    listeners.forEach((push) => push(snapshot));
  }
  function stale(id: string, reason: InventoryObservation["reason"] = "agent.unavailable") {
    const old = retained[id];
    if (old) retained[id] = { ...old, inventory: { ...old.inventory, stale: true, reason } };
  }
  function report(error: unknown) { window.dispatchEvent(new CustomEvent("docklet:adapter-error", { detail: error })); }
  async function refresh(current: number) {
    try {
      await requireCapability("subscribeMetrics");
      const hosts = await listHosts();
      if (current !== generation) return;
      const ids = new Set(hosts.map((h) => h.id));
      for (const id of Object.keys(retained)) if (!ids.has(id)) delete retained[id];
      for (const id of Object.keys(containers)) if (!ids.has(id.split("~")[0]!)) delete containers[id];
      for (const [id, socket] of sockets) if (!ids.has(id)) { sockets.delete(id); socket.close(); }
      for (const h of hosts) {
        if (h.status !== "online") {
          stale(h.id, h.inventory?.reason ?? (h.status === "update-required" ? "agent.update-required" : "agent.unavailable"));
          const socket = sockets.get(h.id); sockets.delete(h.id); socket?.close();
          continue;
        }
        if (!sockets.has(h.id)) {
          const url = new URL(`${baseUrl}/environments/${encodeURIComponent(h.id)}/ws/system/stats`, window.location.href);
          url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
          const socket = socketFactory(url.href);
          sockets.set(h.id, socket);
          socket.onmessage = (event) => {
            if (current !== generation || sockets.get(h.id) !== socket) return;
            try { retained[h.id] = mapHostMetrics(JSON.parse(String(event.data)), h.id, retained[h.id]); emit(); }
            catch (error) { const state = inventoryFailure(h.id, error); stale(h.id, state.reason); onState(h.id, state); emit(); }
          };
          socket.onclose = socket.onerror = () => {
            if (current !== generation || sockets.get(h.id) !== socket) return;
            sockets.delete(h.id); socket.close(); stale(h.id); emit();
          };
        }
      }
      emit();
      await Promise.all(hosts.filter((h) => h.status === "online").map(async (h) => {
        // Resource sorting asks the existing container read to collect real samples.
        try {
          const result = await list("list-containers", h.id, "containers", { sort: "cpuUsage" });
          if (current !== generation) return;
          const samples: MetricsSnapshot["containers"] = {};
          for (const row of result.rows) {
            mapContainer(row, h.id, [], result.observation);
            const sample = row.resourceSample;
            if (!sample || row.labels?.["io.docklet.system"] === "agent") continue;
            if (![sample.cpuPercent, sample.memoryUsageBytes].every((n) => typeof n === "number" && Number.isFinite(n) && n >= 0)) throw new Error("metrics.invalid-sample");
            samples[inventoryId(h.id, row.id)] = { cpu: sample.cpuPercent, memMb: sample.memoryUsageBytes / 1024 ** 2 };
          }
          for (const id of Object.keys(containers)) if (id.startsWith(`${h.id}~`)) delete containers[id];
          Object.assign(containers, samples);
          onState(h.id, result.observation);
        } catch (error) {
          if (current !== generation) return;
          const state = inventoryFailure(h.id, error);
          for (const id of Object.keys(containers)) if (id.startsWith(`${h.id}~`)) delete containers[id];
          stale(h.id, state.reason); onState(h.id, state);
        }
      }));
      if (current === generation) emit();
    } catch (error) { if (current === generation) report(error); }
    finally { if (current === generation && listeners.size) timer = setTimeout(() => void refresh(current), 5000); }
  }
  return (push: (snapshot: MetricsSnapshot) => void) => {
    listeners.add(push);
    if (listeners.size === 1) void refresh(++generation);
    return () => {
      listeners.delete(push);
      if (listeners.size) return;
      ++generation;
      if (timer) clearTimeout(timer);
      for (const socket of sockets.values()) socket.close();
      sockets.clear();
      for (const id of Object.keys(retained)) stale(id);
    };
  };
}
