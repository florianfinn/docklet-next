import { normalizeDockerReference } from "@/domain/docker-reference";
import type { ActionRun, ContainerInfo, Host, HostMetrics, ScannedImage, Stack, Tone } from "@/domain/model";

export type AttentionKind = "unhealthy" | "offline" | "failed" | "critical" | "major" | "disk" | "unknownOrigin";

export interface AttentionItem {
  id: string;
  kind: AttentionKind;
  tone: Tone;
  /** Higher sorts first. */
  weight: number;
  hostId?: string;
  stackId?: string;
  service?: string;
  containerId?: string;
  runId?: string;
  restorePointId?: string;
  values: Record<string, string | number>;
}

interface Input {
  hosts: Host[];
  stacks: Stack[];
  containers: ContainerInfo[];
  runs: ActionRun[];
  images: ScannedImage[];
  metrics: Record<string, HostMetrics> | undefined;
  now: number;
}

const DAY = 86_400_000;

/** Ranks what needs an operator now; every item maps to one direct action. */
export function attentionItems({ hosts, stacks, containers, runs, images, metrics, now }: Input): AttentionItem[] {
  const out: AttentionItem[] = [];
  const offline = new Set(hosts.filter((h) => h.status === "offline").map((h) => h.id));
  for (const s of stacks) {
    if (offline.has(s.hostId)) continue;
    for (const svc of s.services) {
      if (svc.health === "unhealthy") {
        out.push({ id: `u-${s.id}-${svc.name}`, kind: "unhealthy", tone: "err", weight: 100 + svc.restarts24h, hostId: s.hostId, stackId: s.id, service: svc.name, values: { name: svc.name, stack: s.name, restarts: svc.restarts24h } });
      }
      if (svc.update.state === "version" && svc.update.level === "major") {
        out.push({
          id: `m-${s.id}-${svc.name}`,
          kind: "major",
          tone: "orange",
          weight: 40,
          hostId: s.hostId,
          stackId: s.id,
          service: svc.name,
          values: { name: svc.name, current: svc.update.current.version ?? svc.update.current.tag, latest: svc.update.latest.version ?? svc.update.latest.tag },
        });
      }
    }
  }
  for (const h of hosts) {
    if (h.status === "offline") {
      const known = containers.filter((c) => c.hostId === h.id).length;
      out.push({ id: `o-${h.id}`, kind: "offline", tone: "err", weight: 90, hostId: h.id, values: { name: h.name, time: h.lastSeen, containers: known } });
    }
    const m = metrics?.[h.id];
    if (m && h.status === "online") {
      const freeGb = ((100 - m.disk) / 100) * h.diskTotalGb;
      const days = m.diskGrowthGbPerDay > 0 ? Math.round(freeGb / m.diskGrowthGbPerDay) : Infinity;
      if (m.disk >= 80 || days < 60) out.push({ id: `d-${h.id}`, kind: "disk", tone: days < 30 ? "err" : "warn", weight: days < 30 ? 70 : 35, hostId: h.id, values: { host: h.name, percent: Math.round(m.disk), days: Number.isFinite(days) ? days : 999 } });
    }
  }
  const latestPerStack = new Map<string, ActionRun>();
  for (const r of runs) {
    if (!r.stackId || r.status === "running") continue;
    const prev = latestPerStack.get(r.stackId);
    if (!prev || r.startedAt > prev.startedAt) latestPerStack.set(r.stackId, r);
  }
  for (const r of latestPerStack.values()) {
    if (r.status === "failed" && now - new Date(r.startedAt).getTime() < DAY) {
      out.push({ id: `f-${r.id}`, kind: "failed", tone: "err", weight: 80, hostId: r.hostId, stackId: r.stackId, runId: r.id, restorePointId: r.restorePointId, values: { title: r.title } });
    }
  }
  const byRef = new Map<string, { ref: string; count: number; image: ScannedImage }>();
  for (const image of images) if (image.usage === "running" && image.counts.critical > 0) {
    const key = normalizeDockerReference(image.ref) ?? `${image.hostId}:${image.ref}`;
    const previous = byRef.get(key);
    byRef.set(key, { ref: previous?.ref ?? image.ref, count: (previous?.count ?? 0) + image.counts.critical, image: previous?.image ?? image });
  }
  for (const [key, { ref, count, image }] of byRef) {
    out.push({ id: `c-${key}`, kind: "critical", tone: "err", weight: 60, hostId: image.hostId, stackId: image.stackId, values: { image: ref.split("/").pop() ?? ref, count } });
  }
  for (const c of containers) {
    if (c.origin === "unknown" && !offline.has(c.hostId)) out.push({ id: `x-${c.id}`, kind: "unknownOrigin", tone: "info", weight: 20, hostId: c.hostId, containerId: c.id, values: { name: c.name } });
  }
  return out.sort((a, b) => b.weight - a.weight);
}
