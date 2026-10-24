// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { ActionDetailState } from "../adapter";
import type { CodedActionRun } from "../message-code";
import { createSingleFlight } from "./single-flight";

interface Options {
  read: (host: string, id: string, signal: AbortSignal, expanded: boolean) => Promise<CodedActionRun>;
  hasConsumer: () => boolean;
  terminal?: (host: string) => void;
}

/** Full detail has explicit owners. Notification demand shares the transport,
 * with one trailing slot and one start timer, while summaries discard histories. */
export function createActivityTargets({ read, hasConsumer, terminal: terminalMembership = () => {} }: Options) {
  let generation = 0;
  function create(host: string, id: string) {
    const listeners = new Set<(state: ActionDetailState) => void>();
    let state: ActionDetailState = { status: "loading" };
    let nextStart = 0;
    let terminal: string | undefined;
    let membership = false;
    let lastMembership: string | undefined;
    let notificationEpoch = 0;
    let consumedNotification = 0;
    let bypass = false;
    let wake: (() => void) | undefined;
    const flight = createSingleFlight(async (signal) => {
      const wait = listeners.size || bypass ? 0 : Math.max(0, nextStart - Date.now());
      bypass = false;
      if (wait) await new Promise<void>((resolve, reject) => {
        const abort = () => { clearTimeout(timer); wake = undefined; reject(new CancelledError()); };
        const finish = () => { clearTimeout(timer); wake = undefined; signal.removeEventListener("abort", abort); resolve(); };
        const timer = setTimeout(finish, wait); wake = finish;
        signal.addEventListener("abort", abort, { once: true });
        if (signal.aborted) abort();
      });
      if (signal.aborted || (!listeners.size && !hasConsumer())) throw new CancelledError();
      await Promise.resolve();
      if (signal.aborted) throw new CancelledError();
      if (!listeners.size) flight.absorbNotifications();
      bypass = false;
      nextStart = Date.now() + 1_000;
      if (listeners.size && (state.status === "ready" || state.status === "error") && state.run) { state = { status: "ready", run: state.run, refreshing: true }; listeners.forEach((listener) => listener(state)); }
      const discover = membership; membership = false;
      const notification = notificationEpoch;
      try {
        const run = await read(host, id, signal, listeners.size > 0);
        if (run.status === "running" || run.status === "queued") lastMembership = undefined;
        else if (discover) {
          const stamp = `${run.status}:${run.finishedAt ?? ""}`;
          if (stamp !== lastMembership) { lastMembership = stamp; terminalMembership(host); }
        }
        return run;
      }
      catch (error) {
        if (!isCancelledError(error) && !signal.aborted && listeners.size) {
          state = { status: "error", error, run: state.status === "ready" || state.status === "error" ? state.run : undefined }; listeners.forEach((listener) => listener(state));
        }
        throw error;
      } finally { consumedNotification = Math.max(consumedNotification, notification); }
    }, () => generation, { hasConsumer: () => listeners.size > 0 || hasConsumer(), publish: (run) => {
      if (!listeners.size) return;
      state = { status: "ready", run }; listeners.forEach((listener) => listener(state));
    } });
    return { listeners, flight, notification() { notificationEpoch++; }, pendingNotification: () => notificationEpoch > consumedNotification, unavailable() { state = { status: "error", error: new Error("activity.source-unavailable") }; listeners.forEach((listener) => listener(state)); }, membership() { membership = true; }, accept(run: CodedActionRun) { if (listeners.size) { state = { status: "ready", run }; listeners.forEach((listener) => listener(state)); } }, state: () => state, terminal(value?: string) {
      if (value && value !== terminal) { terminal = value; bypass = true; if (wake) { wake(); return true; } }
      else if (!value) terminal = undefined;
    } };
  }
  const entries = new Map<string, ReturnType<typeof create>>();
  const key = (host: string, id: string) => JSON.stringify([host, id]);
  function entry(host: string, id: string) {
    const k = key(host, id); let value = entries.get(k);
    if (!value) { value = create(host, id); entries.set(k, value); }
    return value;
  }
  return {
    observe(host: string, id: string, listener: (state: ActionDetailState) => void) {
      const owner = entry(host, id); owner.listeners.add(listener); listener(owner.state());
      void owner.flight.request().catch(() => undefined);
      return () => {
        if (!owner.listeners.delete(listener)) return;
        if (!owner.listeners.size && !(hasConsumer() && owner.pendingNotification())) { owner.flight.cancel(); if (entries.get(key(host, id)) === owner) entries.delete(key(host, id)); }
      };
    },
    notify(host: string, id: string, terminal?: string, discoverOnTerminal = false) {
      const owner = entry(host, id);
      if (!owner.listeners.size && !hasConsumer()) return;
      owner.notification();
      if (discoverOnTerminal) owner.membership();
      if (owner.terminal(terminal)) return; void owner.flight.notify().catch(() => undefined);
    },
    changed(previous: CodedActionRun[], current: CodedActionRun[], recovered: string[] = []) {
      const prior = new Map(previous.map((run) => [key(run.hostId!, run.id), run]));
      const remaining = new Set(current.map((run) => key(run.hostId!, run.id)));
      for (const [k, owner] of entries) {
        if (!remaining.has(k)) { owner.flight.cancel(); owner.unavailable(); entries.delete(k); continue; }
        if (!owner.listeners.size) continue;
        const run = current.find((row) => key(row.hostId!, row.id) === k)!;
        if (recovered.includes(run.hostId!) || JSON.stringify(prior.get(k)) !== JSON.stringify(run)) void owner.flight.notify().catch(() => undefined);
      }
    },
    accept(run: CodedActionRun) { entries.get(key(run.hostId!, run.id))?.accept(run); },
    dropIdle() { for (const [k, owner] of entries) if (!owner.listeners.size) { owner.flight.cancel(); entries.delete(k); } },
    reset(replay = false) {
      ++generation;
      for (const [k, owner] of entries) {
        owner.flight.cancel(); owner.listeners.forEach((listener) => listener({ status: "loading" }));
        if (replay && owner.listeners.size) queueMicrotask(() => { if (entries.get(k) === owner) void owner.flight.request().catch(() => undefined); });
        else entries.delete(k);
      }
    },
  };
}
