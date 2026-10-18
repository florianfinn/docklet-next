import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useLocale } from "use-intl";

import type { ActionRun, LogLine, MetricsSnapshot } from "@/domain/model";

import type { ActionReadHealth, DockletAdapter } from "./adapter";
import { renderRun, type CodedActionRun } from "./message-code";
import { useAdapter } from "./use-adapter";
import { useActionsQuery } from "./queries";

interface Channel<T> {
  value: T | undefined;
  listeners: Set<() => void>;
  stop?: () => void;
}

type Connect<T> = (adapter: DockletAdapter, push: (value: T) => void) => () => void;

/** Shares one adapter subscription between all components reading a stream. */
function createChannel<T>(connect: Connect<T>, clearOnDisconnect = false) {
  const channels = new WeakMap<DockletAdapter, Channel<T>>();
  const channelFor = (adapter: DockletAdapter) => {
    let ch = channels.get(adapter);
    if (!ch) {
      ch = { value: undefined, listeners: new Set() };
      channels.set(adapter, ch);
    }
    return ch;
  };
  return function useChannel<S>(selector: (value: T | undefined) => S): S {
    const adapter = useAdapter();
    const ch = channelFor(adapter);
    return useSyncExternalStore(
      useCallback((notify: () => void) => {
        ch.listeners.add(notify);
        if (!ch.stop) {
          ch.stop = connect(adapter, (v) => {
            ch.value = v;
            ch.listeners.forEach((l) => l());
          });
        }
        return () => {
          ch.listeners.delete(notify);
          if (ch.listeners.size === 0) {
            ch.stop?.();
            ch.stop = undefined;
            if (clearOnDisconnect) ch.value = undefined;
          }
        };
      }, [adapter, ch]),
      () => selector(ch.value),
      () => selector(ch.value),
    );
  };
}

const useMetricsChannel = createChannel<MetricsSnapshot>((adapter, push) => adapter.subscribeMetrics(push));
const EMPTY_HEALTH: ActionReadHealth = { failures: [] };
const useActionHealthChannel = createChannel<ActionReadHealth>((adapter, push) => adapter.subscribeActionReadHealth?.(push) ?? (() => {}), true);
export function useActionReadHealth(): ActionReadHealth {
  return useActionHealthChannel((health) => health ?? EMPTY_HEALTH);
}

const useActionsChannel = createChannel<CodedActionRun[]>((adapter, push) => adapter.subscribeActions(push), true);

/** Resource values with selectors that keep re-renders narrow. */
export function useMetrics<S>(selector: (snapshot: MetricsSnapshot | undefined) => S): S {
  return useMetricsChannel(selector);
}

const EMPTY_RUNS: CodedActionRun[] = [];

/** Run codes rendered in the interface language. */
export function useActionRuns(): ActionRun[] {
  const runs = useActionsChannel((r) => r);
  const query = useActionsQuery();
  const locale = useLocale();
  const chosen = runs ?? query.data ?? EMPTY_RUNS;
  return useMemo(() => chosen.map((r) => renderRun(r, locale)), [chosen, locale]);
}

export function useActionRun(id: string | undefined): ActionRun | undefined {
  const runs = useActionRuns();
  return id ? runs.find((run) => run.id === id) : undefined;
}

export function useContainerLogs(containerId: string, paused: boolean): LogLine[] {
  const adapter = useAdapter();
  const [lines, setLines] = useState<LogLine[]>([]);
  useEffect(() => {
    if (paused) return;
    return adapter.subscribeContainerLogs(containerId, setLines);
  }, [adapter, containerId, paused]);
  return lines;
}

export function useStackLogs(stackId: string, paused: boolean): LogLine[] {
  const adapter = useAdapter();
  const [lines, setLines] = useState<LogLine[]>([]);
  useEffect(() => {
    if (paused) return;
    return adapter.subscribeLogs(stackId, setLines);
  }, [adapter, stackId, paused]);
  return lines;
}
