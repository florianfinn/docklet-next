import { useSyncExternalStore } from "react";

type Listener = () => void;

export interface Store<T> {
  get(): T;
  set(next: T | ((prev: T) => T)): void;
  subscribe(listener: Listener): () => void;
  reset(): void;
}

interface PersistOptions<T> {
  key: string;
  version: number;
  /** Merges a stored value into defaults so new fields survive schema growth. */
  merge?: (stored: unknown, defaults: T) => T;
}

/** Minimal external store; optional versioned localStorage persistence. */
export function createStore<T>(initial: T, persist?: PersistOptions<T>): Store<T> {
  const listeners = new Set<Listener>();
  let state = persist ? load(initial, persist) : initial;

  return {
    get: () => state,
    set(next) {
      const value = typeof next === "function" ? (next as (prev: T) => T)(state) : next;
      if (Object.is(value, state)) return;
      state = value;
      if (persist) save(persist, value);
      listeners.forEach((l) => l());
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    reset() {
      state = initial;
      if (persist) save(persist, initial);
      listeners.forEach((l) => l());
    },
  };
}

export function useStore<T, S>(store: Store<T>, selector: (state: T) => S): S {
  return useSyncExternalStore(
    store.subscribe,
    () => selector(store.get()),
    () => selector(store.get()),
  );
}

function load<T>(initial: T, persist: PersistOptions<T>): T {
  if (typeof localStorage === "undefined") return initial;
  try {
    const raw = localStorage.getItem(persist.key);
    if (!raw) return initial;
    const parsed = JSON.parse(raw) as { v?: number; data?: unknown };
    if (parsed.v !== persist.version) return initial;
    return persist.merge ? persist.merge(parsed.data, initial) : (parsed.data as T);
  } catch {
    return initial;
  }
}

function save<T>(persist: PersistOptions<T>, value: T): void {
  if (typeof localStorage === "undefined") return;
  try {
    localStorage.setItem(persist.key, JSON.stringify({ v: persist.version, data: value }));
  } catch {
    // Storage may be full or blocked; the in-memory state stays authoritative.
  }
}

/** Like mergeShallow, but drops stored keys the current schema no longer knows. */
export function mergeKnown<T extends object>(stored: unknown, defaults: T): T {
  const merged = mergeShallow(stored, defaults) as Record<string, unknown>;
  return Object.fromEntries(Object.keys(defaults).map((k) => [k, merged[k]])) as T;
}

/** Shallow object merge for persisted settings objects. */
export function mergeShallow<T extends object>(stored: unknown, defaults: T): T {
  if (!stored || typeof stored !== "object") return defaults;
  return { ...defaults, ...(stored as Partial<T>) };
}
