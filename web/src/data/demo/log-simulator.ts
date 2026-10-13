import type { LogLine } from "@/domain/model";

import { FALLBACK_TEMPLATES, LOG_TEMPLATES } from "./fixtures/logs";
import { simulationStore } from "./simulation";

const MAX_LINES = 400;
const TICK_MS = 1700;

/** A log stream: a stack with its services, or one container outside Compose as a single "service". */
export interface LogSource {
  id: string;
  services: { name: string; state: string }[];
}

/** Per-source multiplexed log buffers with synthetic lines. */
export class LogSimulator {
  private readonly buffers = new Map<string, LogLine[]>();
  private readonly listeners = new Map<string, Set<(lines: LogLine[]) => void>>();
  private timer: ReturnType<typeof setInterval> | undefined;
  private seq = 0;

  private readonly stacks: () => LogSource[];

  constructor(stacks: () => LogSource[]) {
    this.stacks = stacks;
  }

  subscribe(stackId: string, listener: (lines: LogLine[]) => void): () => void {
    if (!this.buffers.has(stackId)) this.buffers.set(stackId, this.seed(stackId));
    const set = this.listeners.get(stackId) ?? new Set();
    set.add(listener);
    this.listeners.set(stackId, set);
    listener(this.buffers.get(stackId) ?? []);
    this.ensureTimer();
    return () => {
      set.delete(listener);
      if ([...this.listeners.values()].every((s) => s.size === 0) && this.timer) {
        clearInterval(this.timer);
        this.timer = undefined;
      }
    };
  }

  /** Appends lines produced by actions, e.g. a restart or failed health check. */
  append(stackId: string, service: string, level: LogLine["level"], message: string): void {
    const buf = this.buffers.get(stackId);
    if (!buf) return;
    this.push(stackId, [...buf, this.line(service, level, message, new Date())]);
  }

  private seed(stackId: string): LogLine[] {
    const stack = this.stacks().find((s) => s.id === stackId);
    if (!stack) return [];
    const lines: LogLine[] = [];
    const now = Date.now();
    for (let i = 36; i > 0; i--) {
      const svc = stack.services[i % stack.services.length];
      if (!svc) continue;
      const tpl = this.pick(svc.name);
      lines.push(this.line(svc.name, tpl[0], tpl[1], new Date(now - i * 47_000)));
    }
    return lines;
  }

  private ensureTimer(): void {
    if (this.timer) return;
    this.timer = setInterval(() => {
      if (!simulationStore.get().live) return;
      for (const [stackId, set] of this.listeners) {
        if (set.size === 0) continue;
        const stack = this.stacks().find((s) => s.id === stackId);
        const running = stack?.services.filter((s) => s.state === "running") ?? [];
        const svc = running[Math.floor(Math.random() * running.length)];
        if (!svc) continue;
        const tpl = this.pick(svc.name);
        const buf = this.buffers.get(stackId) ?? [];
        this.push(stackId, [...buf, this.line(svc.name, tpl[0], tpl[1], new Date())]);
      }
    }, TICK_MS);
  }

  private push(stackId: string, lines: LogLine[]): void {
    const next = lines.slice(-MAX_LINES);
    this.buffers.set(stackId, next);
    this.listeners.get(stackId)?.forEach((l) => l(next));
  }

  private pick(service: string): [LogLine["level"], string] {
    const list = LOG_TEMPLATES[service] ?? FALLBACK_TEMPLATES;
    return list[Math.floor(Math.random() * list.length)] ?? ["info", "…"];
  }

  private line(service: string, level: LogLine["level"], message: string, at: Date): LogLine {
    return { id: `l${++this.seq}`, ts: at.toISOString(), service, level, message };
  }
}
