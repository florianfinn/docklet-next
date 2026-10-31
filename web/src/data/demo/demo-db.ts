import { sameImageReference } from "@/domain/docker-reference";
import { pruneEvents } from "@/domain/activity-history";
import { parse } from "yaml";

import { parseCompose, splitImage } from "@/domain/compose/model";
import type {
  ActionRun,
  ContainerInfo,
  Host,
  ImageVersion,
  OpsEvent,
  RestorePoint,
  ScheduledJob,
  SecurityOverview,
  Service,
  Stack,
  Topology,
} from "@/domain/model";

import type { HubSettings } from "@/domain/settings";
import { withoutSystem } from "@/domain/system";
import type { BackupPolicy, BackupRun } from "@/domain/volumes";

import type { Topic } from "../adapter";
import { FileStore } from "./file-store";
import { BACKUP_POLICIES, BACKUP_RUNS } from "./fixtures/backups";
import { COMPOSE, ENV_FILES } from "./fixtures/compose-files";
import { STANDALONE } from "./fixtures/containers";
import { EVENTS, SCHEDULE, SEED_RESTORE_POINTS } from "./fixtures/events";
import { HOSTS } from "./fixtures/hosts";
import { SECURITY } from "./fixtures/security";
import { SETTINGS } from "./fixtures/settings";
import { NOTIFICATION_TARGETS } from "./fixtures/notifications";
import type { NotificationTarget } from "@/domain/notifications";
import type { AutoHealSettings } from "@/domain/auto-heal";
import { STACKS } from "./fixtures/stacks";
import { agentContainer, agentInsideProject, agentReport, agentStack } from "./fixtures/system";
import { TOPOLOGY } from "./fixtures/topology";

const SECRET_KEY = /PASS|SECRET|TOKEN|KEY/i;

/** Mutable in-memory database behind the demo adapter. */
export class DemoDb {
  hosts: Host[] = structuredClone(HOSTS).map((h) => (h.status === "pending" ? h : { ...h, agentReport: agentReport(h, h.lastSeen) }));
  stacks: Stack[] = structuredClone(STACKS);
  standalone: ContainerInfo[] = structuredClone(STANDALONE);
  compose: Record<string, string> = { ...COMPOSE };
  env: Record<string, string> = { ...ENV_FILES };
  restorePoints: RestorePoint[] = structuredClone(SEED_RESTORE_POINTS);
  events: OpsEvent[] = structuredClone(EVENTS);
  schedule: ScheduledJob[] = structuredClone(SCHEDULE);
  security: SecurityOverview = structuredClone(SECURITY);
  topology: Topology = structuredClone(TOPOLOGY);
  /** Containers disconnected from their last network; they report no networks instead of their defaults. */
  isolated = new Set<string>();
  settings: HubSettings = structuredClone(SETTINGS);
  notificationTargets: NotificationTarget[] = structuredClone(NOTIFICATION_TARGETS);
  /** Saved Auto Heal settings per host; unset hosts read the fixture values. */
  autoHeal = new Map<string, AutoHealSettings>();
  files = new FileStore();
  /** Volumes Docker knows without a mount in the fixtures: seeded leftovers and ones created in this session. */
  createdVolumes = new Map<string, { name: string; hostId: string; createdAt: string; labels: Record<string, string>; seeded?: boolean }>();
  backupPolicies = new Map<string, BackupPolicy>(Object.entries(structuredClone(BACKUP_POLICIES)));
  backupRuns: BackupRun[] = structuredClone(BACKUP_RUNS);

  private readonly changeListeners = new Set<(topics: Topic[]) => void>();
  private seq = 100;

  nextId(prefix: string): string {
    return `${prefix}-${++this.seq}`;
  }

  host(id: string | undefined): Host | undefined {
    return this.hosts.find((h) => h.id === id);
  }

  stack(id: string): Stack | undefined {
    return this.stacks.find((s) => s.id === id);
  }

  patchStack(id: string, fn: (s: Stack) => Stack): void {
    this.stacks = this.stacks.map((s) => (s.id === id ? fn(s) : s));
  }

  patchService(stackId: string, name: string, fn: (s: Service) => Service): void {
    this.patchStack(stackId, (s) => ({ ...s, services: s.services.map((x) => (x.name === name ? fn(x) : x)) }));
  }

  patchContainer(id: string, fn: (c: ContainerInfo) => ContainerInfo): void {
    this.standalone = this.standalone.map((c) => (c.id === id ? fn(c) : c));
  }

  /** Stacks as discovery finds them, including the agent's project when it sits in the project directory. */
  discoveredStacks(): Stack[] {
    const agents = this.hosts.filter((h) => h.status !== "pending" && agentInsideProject(h)).map(agentStack);
    return [...this.stacks.filter(stack => this.host(stack.hostId)).map((stack) => this.stoppedDefinition(stack)), ...agents];
  }

  /** Removed containers leave definition services; invalid text never drops the stack. */
  private stoppedDefinition(stack: Stack): Stack {
    if (!stack.services.length || !stack.services.every((service) => service.containersRemoved)) return stack;
    const parsed = parseCompose(this.compose[stack.id] ?? "");
    if (!parsed.ok) return { ...stack, reportedStatus: "stopped", definitionHint: "definition-invalid", services: [] };
    const services = parsed.model.services.map((model): Service => {
      const prior = stack.services.find((service) => service.name === model.key);
      const image = splitImage(model.image);
      return { ...prior, name: model.key, image, state: "stopped", health: "none", containersRemoved: true, containerId: undefined,
        ports: [], networks: model.networks, restartPolicy: model.restart ?? "no", memLimitMb: prior?.memLimitMb ?? 0,
        restarts24h: 0, update: prior?.update ?? { state: "unchecked", current: image } };
    });
    return { ...stack, reportedStatus: "stopped", definitionHint: undefined, services };
  }

  /** Every container Docker reports, including Docklet's own agent on each connected host. */
  discoveredContainers(): ContainerInfo[] {
    return [...this.workloadContainers(), ...this.hosts.filter((h) => h.status !== "pending").map(agentContainer)];
  }

  /** User workloads only: the system label rule removes the agent from every list and count. */
  containers(): ContainerInfo[] {
    return withoutSystem(this.discoveredContainers()).filter(container => this.host(container.hostId));
  }

  /** Network names from the topology attachments, which connect and disconnect change. */
  private networkNames(containerId: string): string[] | undefined {
    const names = this.topology.attachments.filter((a) => a.containerId === containerId).map((a) => this.topology.networks.find((n) => n.id === a.networkId)?.name ?? a.networkId);
    return names.length || this.isolated.has(containerId) ? names : undefined;
  }

  /** Compose services projected as containers, plus standalone containers. */
  private workloadContainers(): ContainerInfo[] {
    const fromStacks = this.stacks.flatMap((stack) => {
      const details = serviceDetails(this.compose[stack.id] ?? "");
      return stack.services.filter((svc) => !svc.containersRemoved).map((svc): ContainerInfo => {
        const d = details[svc.name];
        return {
          id: containerId(stack.id, svc.name),
          name: `${stack.id}-${svc.name}-1`,
          hostId: stack.hostId,
          stackId: stack.id,
          serviceName: svc.name,
          origin: "compose",
          originSource: "detected",
          originNote: `Compose-Projekt ${stack.id}`,
          image: svc.image,
          state: svc.state,
          health: svc.health,
          ports: svc.ports,
          networks: this.networkNames(containerId(stack.id, svc.name)) ?? svc.networks,
          mounts: d?.mounts ?? [],
          env: d?.env ?? {},
          restartPolicy: svc.restartPolicy,
          memLimitMb: svc.memLimitMb,
          startedAt: svc.startedAt,
          update: svc.update,
        };
      });
    });
    return [...fromStacks, ...this.standalone.map((c) => ({ ...c, originSource: c.originSource ?? "detected" as const, networks: this.networkNames(c.id) ?? c.networks }))];
  }

  pushEvent(event: Omit<OpsEvent, "id" | "at">): void {
    this.events = pruneEvents([{ ...event, id: this.nextId("e"), at: new Date().toISOString() }, ...this.events], this.settings.activity.maxEvents);
    this.changed("events");
  }

  saveRestorePoint(stack: Stack, reason: string): RestorePoint {
    const rp: RestorePoint = {
      id: this.nextId("rp"),
      stackId: stack.id,
      createdAt: new Date().toISOString(),
      reason,
      images: stack.services.map((s) => ({ service: s.name, image: s.image })),
      composeSnapshot: this.compose[stack.id] ?? "",
    };
    this.restorePoints = [rp, ...this.restorePoints];
    return rp;
  }

  /** Rebuilds a stack's services from its compose text after a deploy. */
  syncServicesFromCompose(stackId: string): void {
    const parsed = parseCompose(this.compose[stackId] ?? "");
    if (!parsed.ok) return;
    this.patchStack(stackId, (stack) => {
      const services = parsed.model.services.map((m): Service => {
        const prev = stack.services.find((s) => s.name === m.key);
        const { repository, tag } = splitImage(m.image);
        const sameImage = prev && sameImageReference(`${prev.image.repository}:${prev.image.tag}`, m.image);
        const image: ImageVersion = sameImage ? prev.image : { repository, tag, version: /\d/.test(tag) ? tag.replace(/^v/, "").split("-")[0] : undefined, published: new Date().toISOString() };
        return {
          name: m.key,
          image,
          state: "running",
          health: m.health.enabled ? "healthy" : "none",
          ports: m.ports.filter((p) => p.editable).map((p) => ({ host: p.host || undefined, container: p.container })),
          networks: m.networks.length ? m.networks : [`${stackId}_default`],
          restartPolicy: m.restart || "no",
          memLimitMb: prev?.memLimitMb ?? 512,
          startedAt: new Date().toISOString(),
          restarts24h: 0,
          update: sameImage && prev ? prev.update : { state: "current", current: image, checkedAt: new Date().toISOString(), registry: "Docker Hub · anonym" },
        };
      });
      return { ...stack, services, lastDeployAt: new Date().toISOString() };
    });
  }

  subscribe(listener: (topics: Topic[]) => void): () => void {
    this.changeListeners.add(listener);
    return () => this.changeListeners.delete(listener);
  }

  changed(...topics: Topic[]): void {
    this.changeListeners.forEach((l) => l(topics));
  }

  deploymentsFor(stackId: string, runs: ActionRun[]): ActionRun[] {
    return runs.filter((r) => r.stackId === stackId && ["deploy", "redeploy", "update", "restore"].includes(r.kind));
  }
}

export function containerId(stackId: string, service: string): string {
  return `c-${stackId}-${service}`;
}

function serviceDetails(compose: string): Record<string, { mounts: string[]; env: Record<string, string> }> {
  try {
    const js = parse(compose) as { services?: Record<string, { volumes?: unknown; environment?: unknown }> } | null;
    const out: Record<string, { mounts: string[]; env: Record<string, string> }> = {};
    for (const [name, svc] of Object.entries(js?.services ?? {})) {
      const mounts = Array.isArray(svc?.volumes) ? svc.volumes.filter((v): v is string => typeof v === "string") : [];
      const env: Record<string, string> = {};
      const raw = svc?.environment;
      if (Array.isArray(raw)) {
        for (const e of raw) {
          const [k, ...v] = String(e).split("=");
          if (k) env[k] = v.join("=");
        }
      } else if (raw && typeof raw === "object") {
        for (const [k, v] of Object.entries(raw)) env[k] = String(v ?? "");
      }
      for (const k of Object.keys(env)) if (SECRET_KEY.test(k) || env[k]?.includes("${")) env[k] = "••••••";
      out[name] = { mounts, env };
    }
    return out;
  } catch {
    return {};
  }
}
