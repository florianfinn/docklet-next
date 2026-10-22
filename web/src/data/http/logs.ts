// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original shared, bounded WS log lifetime owner.

import type { LogLine } from "@/domain/model";
import type { CapabilityDescriptor, ResourceRef } from "../capability";
import { gapLine, LogBuffer, registerLogLifetime, setLogDelivery, type LogStatus } from "../log-buffer";
import { logAddress, mapLog, type LogKind } from "./log-mapping";
import type { MetricsSocket } from "./metrics";

type Listener = (lines: LogLine[]) => void;
type LogSocket = MetricsSocket & { onopen?: ((event: Event) => void) | null };
interface Options {
  baseUrl: string;
  socket: (url: string) => MetricsSocket;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  authenticated: () => boolean;
  checkIdentity: (signal: AbortSignal) => Promise<boolean>;
}
interface Channel {
  address: ReturnType<typeof logAddress>;
  kind: LogKind;
  id: string;
  listeners: Set<Listener>;
  buffer: LogBuffer;
  published: LogLine[];
  status: LogStatus;
  reconnect: boolean;
  active: boolean;
  epoch: number;
  attempt: number;
  lastSeq?: number;
  failures: number;
  healthySince?: number;
  frame?: number;
  timer?: ReturnType<typeof setTimeout>;
  socket?: LogSocket;
  check?: AbortController;
}
const HEALTHY_CONNECTION_MS = 30000;
const DELAYS = [1000, 2000, 5000, 10000, 30000];

export function createRuntimeLogs(options: Options) {
  const channels = new Map<string, Channel>();
  const history = new Map<string, true>();
  const lifetimeListeners = new Set<() => void>();
  let generation = 0, attempt = 0;
  const online = () => navigator.onLine !== false;
  const current = (ch: Channel, epoch = ch.epoch) => ch.active && ch.epoch === epoch;
  function cancelFrame(ch: Channel) { if (ch.frame !== undefined) cancelAnimationFrame(ch.frame); ch.frame = undefined; }
  function close(ch: Channel) {
    ch.epoch++;
    if (ch.timer !== undefined) clearTimeout(ch.timer);
    ch.timer = undefined;
    ch.check?.abort(); ch.check = undefined;
    const socket = ch.socket; ch.socket = undefined;
    if (socket) { socket.onopen = socket.onmessage = socket.onerror = socket.onclose = null; socket.close(); }
  }
  function scheduleFlush(ch: Channel) {
    if (!ch.active || ch.frame !== undefined) return;
    ch.frame = requestAnimationFrame(() => {
      ch.frame = undefined;
      if (!ch.active) return;
      ch.published = ch.buffer.snapshot({ status: ch.status, reconnect: ch.reconnect, retry: () => retry(ch) });
      ch.listeners.forEach((push) => push(ch.published));
    });
  }
  function state(ch: Channel, status: LogStatus) { ch.status = status; scheduleFlush(ch); }
  function retry(ch: Channel) {
    if (!ch.active) return;
    close(ch); ch.failures = 0;
    void connect(ch);
  }
  function later(ch: Channel) {
    if (!ch.active) return;
    if (!options.authenticated()) { state(ch, "unauthenticated"); return; }
    if (!online()) { state(ch, "offline"); return; }
    const delay = DELAYS[ch.failures++];
    if (delay === undefined) { state(ch, "error"); return; }
    state(ch, "retrying");
    ch.timer = setTimeout(() => { ch.timer = undefined; void connect(ch); }, delay);
  }
  async function disconnected(ch: Channel, socket: MetricsSocket) {
    if (!current(ch) || ch.socket !== socket) return;
    // Valid delivery must survive a continuous interval before replenishing retries.
    // A single frame followed by a quick close is still a flapping connection.
    if (ch.healthySince !== undefined && performance.now() - ch.healthySince >= HEALTHY_CONNECTION_MS) ch.failures = 0;
    close(ch);
    if (!online()) { state(ch, "offline"); return; }
    const epoch = ch.epoch, controller = new AbortController();
    ch.check = controller;
    // A closed WS is not proof of logout. Recheck the existing hub identity
    // before retrying, with a bounded request lifetime owned by this channel.
    ch.timer = setTimeout(() => controller.abort(), 10000);
    try {
      const authenticated = await options.checkIdentity(controller.signal);
      if (!current(ch, epoch)) return;
      if (!authenticated) { state(ch, "unauthenticated"); return; }
    } catch { /* A transport failure is retried within the same bounded budget. */ }
    finally {
      if (current(ch, epoch)) {
        if (ch.timer !== undefined) clearTimeout(ch.timer);
        ch.timer = undefined; ch.check = undefined;
      }
    }
    if (current(ch, epoch)) later(ch);
  }
  async function connect(ch: Channel) {
    if (!current(ch) || ch.socket || ch.check) return;
    if (!options.authenticated()) { state(ch, "unauthenticated"); return; }
    if (!online()) { state(ch, "offline"); return; }
    const epoch = ch.epoch;
    state(ch, "connecting");
    let capability: CapabilityDescriptor;
    try { capability = await options.capability(ch.kind === "stack" ? "subscribeLogs" : "subscribeContainerLogs", { kind: ch.kind, id: ch.id }); }
    catch { if (current(ch, epoch)) state(ch, "unavailable"); return; }
    if (!current(ch, epoch) || !options.authenticated()) return;
    if (!capability.available) { state(ch, "unavailable"); return; }
    ch.attempt = ++attempt;
    ch.reconnect = history.has(ch.address.key);
    history.delete(ch.address.key); history.set(ch.address.key, true);
    if (history.size > 256) history.delete(history.keys().next().value!);
    ch.lastSeq = undefined;
    ch.healthySince = undefined;
    ch.buffer.clear();
    state(ch, "connecting");
    let socket: LogSocket;
    try { socket = options.socket(ch.address.url); }
    catch { later(ch); return; }
    ch.socket = socket;
    socket.onopen = () => { if (current(ch, epoch) && ch.socket === socket) state(ch, "live"); };
    socket.onmessage = (event) => {
      if (!current(ch, epoch) || ch.socket !== socket || !options.authenticated()) return;
      try {
        if (typeof event.data !== "string") throw new Error("logs.invalid-frame");
        const value: unknown = JSON.parse(event.data);
        const values = Array.isArray(value) ? value : [value];
        for (const value of values) {
          const line = mapLog(value, ch.address.key, ch.attempt, ch.kind === "container" ? ch.address.raw : "", () => new Date().toISOString());
          if (ch.lastSeq !== undefined) {
            if (line.seq! <= ch.lastSeq) throw new Error("logs.invalid-sequence");
            if (line.seq! > ch.lastSeq + 1) ch.buffer.append(gapLine(`${line.id}:gap`, line.ts, line.seq! - ch.lastSeq - 1));
          }
          ch.lastSeq = line.seq;
          ch.buffer.append(line);
        }
        if (values.length > 0) ch.healthySince ??= performance.now();
        state(ch, "live");
      } catch { close(ch); state(ch, "error"); }
    };
    socket.onclose = socket.onerror = () => { void disconnected(ch, socket); };
  }
  function network() {
    for (const ch of channels.values()) {
      close(ch);
      if (online()) { ch.failures = 0; void connect(ch); } else state(ch, "offline");
    }
  }
  function subscribe(kind: LogKind, id: string, push: Listener) {
    const address = logAddress(options.baseUrl, kind, id);
    let ch = channels.get(address.key);
    if (!ch) {
      ch = { address, kind, id, listeners: new Set(), buffer: new LogBuffer(), published: [], status: "connecting", reconnect: false, active: true, epoch: 0, attempt: 0, failures: 0 };
      setLogDelivery(ch.published, { status: "connecting", reconnect: false, dropped: 0 });
      channels.set(address.key, ch);
      if (channels.size === 1) { window.addEventListener("online", network); window.addEventListener("offline", network); }
      void connect(ch);
    }
    const channel = ch;
    channel.listeners.add(push); push(channel.published);
    return () => {
      channel.listeners.delete(push);
      if (channel.listeners.size) return;
      channel.active = false; close(channel); cancelFrame(channel); channel.buffer.clear(); channel.published = [];
      if (channels.get(address.key) === channel) channels.delete(address.key);
      if (!channels.size) { window.removeEventListener("online", network); window.removeEventListener("offline", network); }
    };
  }
  const subscribeLogs = (id: string, push: Listener) => subscribe("stack", id, push);
  const subscribeContainerLogs = (id: string, push: Listener) => subscribe("container", id, push);
  const lifetime = { version: () => generation, listen: (notify: () => void) => { lifetimeListeners.add(notify); return () => { lifetimeListeners.delete(notify); }; } };
  registerLogLifetime(subscribeLogs, lifetime); registerLogLifetime(subscribeContainerLogs, lifetime);
  return {
    subscribeLogs, subscribeContainerLogs,
    reset() {
      ++generation;
      for (const ch of channels.values()) {
        ch.active = false; close(ch); cancelFrame(ch); ch.buffer.clear();
        ch.published = setLogDelivery([], { status: "unauthenticated", reconnect: false, dropped: 0 });
        ch.listeners.forEach((push) => push(ch.published)); ch.listeners.clear();
      }
      channels.clear(); history.clear();
      window.removeEventListener("online", network); window.removeEventListener("offline", network);
      lifetimeListeners.forEach((notify) => notify());
    },
  };
}
