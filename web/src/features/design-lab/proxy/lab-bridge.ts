// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useEffect } from "react";
import { createStore, useStore } from "@/shared/lib/store";
import { proxyStore, routeName, validRouteName, type ProxyKind, type ProxyRoute } from "./model";
import { routeFromText, routeText } from "./route-text";

export interface LabProxy {
  kind: ProxyKind; container: string; image: string; reachable: boolean; version?: string; reload: string;
  capabilities: { dnsChallenge: boolean; labels: boolean; publicAcme: boolean; localCa: boolean };
  hostnames: string[]; middlewares?: string[];
}
export interface LabInfo { proxies: LabProxy[]; backends: string[] }
export interface LabProbe {
  host?: string; scheme?: string; status?: number; ms?: number; backend?: string; location?: string;
  headers?: Record<string, string>; certificate?: { subject?: unknown; issuer?: unknown; validTo?: string; altNames?: string }; error?: string;
}
export interface LabResult {
  ok: boolean; stage?: string; error?: string; warnings?: unknown; router?: unknown; service?: unknown; changed?: boolean; errors?: unknown; probe?: LabProbe[];
}
export interface LabRecord { names: string[]; text: string; result: LabResult }
interface LabState { info: LabInfo | null; records: Record<string, LabRecord>; busy: boolean }
export const labStore = createStore<LabState>({ info: null, records: {}, busy: false });
let pending: Promise<void> | undefined;
export async function discoverLab(): Promise<void> {
  pending ??= (async () => {
    try {
      const response = await fetch("/lab-api/info", { signal: AbortSignal.timeout(3000) });
      const info = await response.json() as LabInfo;
      if (!response.ok || !Array.isArray(info.proxies) || !Array.isArray(info.backends) || info.proxies.some((p) => !["caddy", "traefik"].includes(p.kind) || !Array.isArray(p.hostnames) || !p.capabilities)) throw new Error("info");
      labStore.set((s) => ({ ...s, info }));
    } catch { labStore.set((s) => ({ ...s, info: null })); }
  })().finally(() => { pending = undefined; });
  return pending;
}
export function useLab() {
  const state = useStore(labStore, (s) => s);
  useEffect(() => { if (!labStore.get().info) void discoverLab(); }, []);
  return state;
}
async function request(kind: ProxyKind, name: string, method: "PUT" | "DELETE", body?: unknown): Promise<LabResult> {
  try {
    const response = await fetch("/lab-api/" + kind + "/routes/" + encodeURIComponent(name), { method, headers: { "Content-Type": "application/json" }, ...(body ? { body: JSON.stringify(body) } : {}), signal: AbortSignal.timeout(20000) });
    const result = await response.json() as LabResult;
    return { ...result, ok: response.ok && result.ok === true };
  } catch (error) { return { ok: false, stage: "connection", error: error instanceof Error ? error.message : String(error) }; }
}
export function labResultFailed(result: LabResult): boolean {
  return !result.ok || !!(Array.isArray(result.errors) ? result.errors.length : result.errors && Object.keys(result.errors).length) || !!result.probe?.some((p) => p.error || (p.status ?? 0) >= 400);
}
export function labRouteStatus(route: ProxyRoute, record?: LabRecord): "labApplied" | "labError" | "labNotApplied" {
  if (!record) return "labNotApplied";
  if (labResultFailed(record.result)) return "labError";
  try { return routeText(routeFromText(record.text, route)) === routeText(route) ? "labApplied" : "labNotApplied"; } catch { return "labError"; }
}
export async function applyLabRoute(route: ProxyRoute, text = routeText(route)): Promise<LabResult> {
  const kind = proxyStore.get().connections[route.proxy]?.lab;
  if (!kind || !validRouteName(routeName(route))) return { ok: false, stage: "policy", error: "route-name" };
  const previous = labStore.get().records[route.id];
  const name = routeName(route);
  const result = await request(kind, name, "PUT", { text, probe: { domain: route.domain, path: route.path || "/", tls: route.tls !== "off", count: route.backends.length ? 4 : 1 } });
  const saved = proxyStore.get().routes.find((r) => r.id === route.id && r.proxy === route.proxy);
  const names = [...new Set([...(previous?.names ?? []), ...(saved?.labNames ?? []), ...(saved ? [routeName(saved)] : []), name])];
  if (result.ok) {
    for (const old of names.filter((n) => n !== name)) {
      const removed = await request(kind, old, "DELETE");
      if (!removed.ok) { result.ok = false; result.stage = removed.stage ?? "delete"; result.error = removed.error; break; }
      names.splice(names.indexOf(old), 1);
    }
  }
  proxyStore.set((s) => ({ ...s, routes: s.routes.map((r) => r.id === route.id ? { ...r, labNames: names } : r) }));
  labStore.set((s) => ({ ...s, records: { ...s.records, [route.id]: { names, text, result } } }));
  return result;
}
export async function deleteLabRoute(route: ProxyRoute): Promise<LabResult> {
  const kind = proxyStore.get().connections[route.proxy]?.lab;
  if (!kind) return { ok: true };
  const record = labStore.get().records[route.id];
  // Also delete the saved name after a browser reload, when observations are intentionally unknown.
  for (const name of new Set([...(record?.names ?? []), ...(proxyStore.get().routes.find((r) => r.id === route.id)?.labNames ?? route.labNames ?? []), routeName(route)])) {
    const result = await request(kind, name, "DELETE");
    if (!result.ok) {
      labStore.set((s) => ({ ...s, records: { ...s.records, [route.id]: { names: record?.names ?? [name], text: record?.text ?? "", result } } }));
      return result;
    }
  }
  labStore.set((s) => ({ ...s, records: Object.fromEntries(Object.entries(s.records).filter(([id]) => id !== route.id)) }));
  return { ok: true };
}
export async function reapplyLab(instance: string) {
  if (labStore.get().busy) return;
  labStore.set((s) => ({ ...s, busy: true }));
  try {
    for (const route of proxyStore.get().routes.filter((r) => r.proxy === instance && r.source === "file")) {
      try { await applyLabRoute(route); }
      catch (error) { labStore.set((s) => ({ ...s, records: { ...s.records, [route.id]: { names: [], text: "", result: { ok: false, stage: "form", error: error instanceof Error ? error.message : String(error) } } } })); }
    }
  } finally { labStore.set((s) => ({ ...s, busy: false })); }
}
