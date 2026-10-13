import type { ContainerMetrics, Host, HostMetrics, MetricsSnapshot } from "@/domain/model";

import { BASELINES } from "./fixtures/stacks";
import { simulationStore } from "./simulation";

const HISTORY = 40;
const TICK_MS = 2000;

const HOST_BASE: Record<string, { cpu: number; mem: number; disk: number; growth: number }> = {
  atlas: { cpu: 27, mem: 44, disk: 71, growth: 9.5 },
  boreal: { cpu: 48, mem: 58, disk: 52, growth: 2.1 },
  delta: { cpu: 17, mem: 39, disk: 81, growth: 0.9 },
  cedar: { cpu: 12, mem: 33, disk: 46, growth: 0.3 },
};

function clamp(v: number, lo = 0, hi = 100): number {
  return Math.min(hi, Math.max(lo, v));
}

function walk(prev: number, base: number, spread: number): number {
  const pull = (base - prev) * 0.25;
  const noise = (Math.random() - 0.5) * spread;
  const spike = Math.random() < 0.06 ? Math.random() * spread * 2.2 : 0;
  return clamp(prev + pull + noise + spike, 1, 99);
}

const NEW_HOST_BASE = { cpu: 9, mem: 22, disk: 18, growth: 0.2 };

/** Synthetic history for a host; hosts added during onboarding start quiet. */
function seedHost(h: Host): HostMetrics {
  const b = HOST_BASE[h.id] ?? NEW_HOST_BASE;
  const cpuHistory: number[] = [];
  const memHistory: number[] = [];
  let cpu = b.cpu;
  let mem = b.mem;
  for (let i = 0; i < HISTORY; i++) {
    cpu = walk(cpu, b.cpu, 9);
    mem = clamp(mem + (Math.random() - 0.5) * 1.5, 5, 95);
    cpuHistory.push(Math.round(cpu));
    memHistory.push(Math.round(mem));
  }
  return { hostId: h.id, cpu: Math.round(cpu), mem: Math.round(mem), disk: b.disk, diskGrowthGbPerDay: b.growth, cpuHistory, memHistory };
}

interface Source {
  hosts: () => Host[];
  /** Container id → running state and host. */
  containers: () => { id: string; hostId: string; running: boolean }[];
}

/** Synthetic live resource values; flagged as simulated in every snapshot. */
export class MetricsSimulator {
  private snapshot: MetricsSnapshot;
  private readonly listeners = new Set<(s: MetricsSnapshot) => void>();
  private timer: ReturnType<typeof setInterval> | undefined;

  private readonly source: Source;

  constructor(source: Source) {
    this.source = source;
    const hosts: Record<string, HostMetrics> = {};
    for (const h of source.hosts()) if (h.status !== "pending") hosts[h.id] = seedHost(h);
    this.snapshot = { at: new Date().toISOString(), simulated: true, hosts, containers: this.containerValues() };
  }

  current(): MetricsSnapshot {
    return this.snapshot;
  }

  subscribe(listener: (s: MetricsSnapshot) => void): () => void {
    this.listeners.add(listener);
    listener(this.snapshot);
    this.ensureTimer();
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0 && this.timer) {
        clearInterval(this.timer);
        this.timer = undefined;
      }
    };
  }

  private ensureTimer(): void {
    if (this.timer) return;
    this.timer = setInterval(() => {
      if (!simulationStore.get().live) return;
      this.tick();
    }, TICK_MS);
  }

  private tick(): void {
    const hosts: Record<string, HostMetrics> = {};
    for (const h of this.source.hosts()) {
      const prev = this.snapshot.hosts[h.id] ?? (h.status === "online" ? seedHost(h) : undefined);
      if (!prev) continue;
      if (h.status !== "online") {
        hosts[h.id] = prev;
        continue;
      }
      const b = HOST_BASE[h.id] ?? NEW_HOST_BASE;
      const cpu = Math.round(walk(prev.cpu, b.cpu, 10));
      const mem = Math.round(clamp(prev.mem + (Math.random() - 0.5) * 1.6, 5, 95));
      const disk = clamp(prev.disk + (b.growth / h.diskTotalGb) * 100 * 0.002, 0, 100);
      hosts[h.id] = {
        ...prev,
        cpu,
        mem,
        disk,
        cpuHistory: [...prev.cpuHistory.slice(1 - HISTORY), cpu],
        memHistory: [...prev.memHistory.slice(1 - HISTORY), mem],
      };
    }
    this.snapshot = { at: new Date().toISOString(), simulated: true, hosts, containers: this.containerValues() };
    this.listeners.forEach((l) => l(this.snapshot));
  }

  private containerValues(): Record<string, ContainerMetrics> {
    const out: Record<string, ContainerMetrics> = {};
    const offline = new Set(this.source.hosts().filter((h) => h.status !== "online").map((h) => h.id));
    for (const c of this.source.containers()) {
      const base = BASELINES[c.id] ?? { cpu: 2, mem: 120 };
      if (!c.running || offline.has(c.hostId)) {
        out[c.id] = { cpu: 0, memMb: 0 };
        continue;
      }
      const prev = this.snapshot?.containers[c.id];
      const cpu = prev ? walk(prev.cpu, base.cpu, Math.max(1, base.cpu * 0.5)) : base.cpu;
      const mem = Math.max(4, (prev?.memMb ?? base.mem) + (Math.random() - 0.5) * base.mem * 0.04);
      out[c.id] = { cpu: Math.round(cpu * 10) / 10, memMb: Math.round(mem) };
    }
    return out;
  }
}
