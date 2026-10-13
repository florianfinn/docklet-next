import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { useLocale } from "use-intl";

import type { ActionRun, LogLine, MetricsSnapshot } from "@/domain/model";

import type { DockletAdapter } from "./adapter";
import { localizeRun } from "@/runtime";
import { useAdapter } from "./use-adapter";

interface Channel<T> {
  value: T | undefined;
  listeners: Set<() => void>;
  stop?: () => void;
}

type Connect<T> = (adapter: DockletAdapter, push: (value: T) => void) => () => void;

/** Shares one adapter subscription between all components reading a stream. */
function createChannel<T>(connect: Connect<T>) {
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
      (notify) => {
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
          }
        };
      },
      () => selector(ch.value),
      () => selector(ch.value),
    );
  };
}

const useMetricsChannel = createChannel<MetricsSnapshot>((adapter, push) => adapter.subscribeMetrics(push));
const useActionsChannel = createChannel<ActionRun[]>((adapter, push) => adapter.subscribeActions(push));

/** Live, simulated resource values; selectors keep re-renders narrow. */
export function useMetrics<S>(selector: (snapshot: MetricsSnapshot | undefined) => S): S {
  return useMetricsChannel(selector);
}

const EMPTY_RUNS: ActionRun[] = [];

/** Runs with their demo texts in the interface language. */
export function useActionRuns(): ActionRun[] {
  const runs = useActionsChannel((r) => r ?? EMPTY_RUNS);
  const locale = useLocale();
  return useMemo(() => (locale === "de" ? runs : runs.map((r) => localizeRun(r, locale))), [runs, locale]);
}

export function useActionRun(id: string | undefined): ActionRun | undefined {
  const run = useActionsChannel((runs) => (id ? runs?.find((r) => r.id === id) : undefined));
  const locale = useLocale();
  return run && localizeRun(run, locale);
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
