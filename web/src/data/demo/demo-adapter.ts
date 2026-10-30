import { sameImageReference } from "@/domain/docker-reference";
import { createDemoRegistries } from "./demo-registries";
import type { ActionKind, ContainerInfo, DeployStepKey, Service, Stack } from "@/domain/model";
import { parseCompose } from "@/domain/compose/model";
import { convertDockerRun } from "@/domain/compose/docker-run";
import { stringify } from "yaml";
import { withoutSystem } from "@/domain/system";
import { normalizeStackLabels } from "@/domain/stack-labels";
import { hasUpdate, imageRef } from "@/domain/updates";

import type { DeployInput, DockletAdapter, StackCommand } from "../adapter";
import { ActionRunner, type RunPlan, type StepPlan } from "./action-runner";
import { composeScript, containerScript, genericScript, type ComposeTarget } from "./run-output";
import type { DemoContext } from "./demo-context";
import { containerId, DemoDb } from "./demo-db";
import { createHostsApi } from "./demo-hosts";
import { createImagesApi } from "./demo-images";
import { createNetworksApi, syncStackNetworks } from "./demo-networks";
import { createSettingsApi } from "./demo-settings";
import { createDemoNotifications } from "./demo-notifications";
import { createVolumesApi } from "./demo-volumes";
import { SEED_RUNS } from "./fixtures/events";
import { simulateImport } from "./import-simulator";
import { LogSimulator } from "./log-simulator";
import { MetricsSimulator } from "./metrics-simulator";
import { encodeDemoEvent, encodeDemoRuns, encodeDemoStackDetail } from "./encode-message";
import type { CodedStackDetail } from "../message-code";
import { registerDemoOptions, takeDemoFailure, type DemoAdapterOptions, type FailAt } from "./options";
import { simulateRead, simulateWrite, simulationStore, takeForcedFailure } from "./simulation";
import { GENERIC_REASON, HUB_REF, unavailable } from "../capability";
import { createDemoHubBackups } from "./demo-hub-backups";

export class DemoCommandError extends Error {
  readonly code: "not-found" | "recreate-not-allowed" | "managed-by-unraid";
  constructor(code: DemoCommandError["code"]) {
    super(code);
    this.code = code;
    this.name = "DemoCommandError";
  }
}

const clone = <T>(v: T): T => structuredClone(v);

/** In-memory adapter: synthetic data, simulated actions, no Docker calls. */
export function createDemoAdapter(options: DemoAdapterOptions = {}): DockletAdapter {
  const db = new DemoDb();
  const composeTarget = (stack: Stack): ComposeTarget => ({ project: stack.id, services: stack.services.map((s) => ({ name: s.name, image: imageRef(s.image) })), hostName: db.host(stack.hostId)?.name ?? stack.hostId });
  const runner = new ActionRunner(options.seedRuns ?? SEED_RUNS, (run) => {
    const stack = run.stackId ? db.stack(run.stackId) : undefined;
    return stack ? composeScript(composeTarget(stack), run.services) : genericScript;
  }, () => db.settings.activity.maxFinishedActions);
  const metrics = new MetricsSimulator({
    hosts: () => db.hosts,
    containers: () => db.containers().map((c) => ({ id: c.id, hostId: c.hostId, running: c.state === "running" })),
  });
  // Containers outside Compose get their own stream, keyed so it never collides with a stack id.
  const logs = new LogSimulator(() => [...db.stacks, ...db.containers().filter((c) => !c.stackId).map((c) => ({ id: `container:${c.id}`, services: [{ name: c.name, state: c.state }] }))]);
  const empty = () => simulationStore.get().empty;
  const ctx: DemoContext = { db, runner, forcedFailure: () => takeForcedFailure() !== "none", demoFailure: (operation) => takeDemoFailure(adapter, operation) };
  const supportedOperations = new Set<string>();

  function failStep(requested: FailAt | undefined): DeployStepKey | undefined {
    const forced = takeForcedFailure();
    const pick = forced !== "none" ? forced : (requested ?? "none");
    return pick === "none" ? undefined : pick;
  }

  function hostLabel(stack: Stack): string {
    return `${stack.name} · ${db.host(stack.hostId)?.name ?? stack.hostId}`;
  }

  function requireStack(id: string): Stack {
    const s = db.stack(id);
    if (!s) throw new DemoCommandError("not-found");
    return s;
  }

  function offlineGuard(stack: Stack, plan: RunPlan): RunPlan {
    const host = db.host(stack.hostId);
    if (host?.status !== "offline") return plan;
    return { ...plan, failAt: "validate", failMessage: `${host.name} ist nicht verbunden – letzter Kontakt ${new Date(host.lastSeen).toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" })}` };
  }

  function unhealthyFailure(stack: Stack, plan: RunPlan): RunPlan {
    if (plan.failAt) return plan;
    const bad = stack.services.find((s) => s.health === "unhealthy");
    if (!bad || !plan.steps.some((s) => s.key === "health")) return plan;
    return { ...plan, failAt: "health", failMessage: `${bad.name} bleibt unhealthy (Healthcheck: timeout)` };
  }

  function runForStack(stack: Stack, kind: ActionKind, title: string, steps: StepPlan[], extra: Partial<RunPlan>): string {
    let plan: RunPlan = {
      kind,
      title,
      target: hostLabel(stack),
      hostId: stack.hostId,
      stackId: stack.id,
      services: extra.services ?? stack.services.map((s) => s.name),
      steps,
      output: composeScript(composeTarget(stack), extra.services),
      ...extra,
      onStart: (run) => {
        db.patchStack(stack.id, (s) => ({ ...s, busyActionId: run.id }));
        db.changed("stacks", "containers");
        extra.onStart?.(run);
      },
      onSuccess: (run) => {
        db.patchStack(stack.id, (s) => ({ ...s, busyActionId: undefined }));
        extra.onSuccess?.(run);
        db.pushEvent({ kind: kind === "update" ? "update" : "deploy", level: "ok", title: `${title} abgeschlossen`, hostId: stack.hostId, stackId: stack.id, actionId: run.id });
        db.changed("stacks", "containers", "topology", "security");
      },
      onFailure: (run, step) => {
        db.patchStack(stack.id, (s) => ({ ...s, busyActionId: undefined }));
        extra.onFailure?.(run, step);
        db.pushEvent({ kind: "deploy", level: "error", title: `${title} fehlgeschlagen: ${step.label}`, hostId: stack.hostId, stackId: stack.id, actionId: run.id });
        db.changed("stacks", "containers");
      },
    };
    plan = offlineGuard(stack, plan);
    if (kind !== "restore") plan = unhealthyFailure(stack, plan);
    return runner.start(plan);
  }

  const setServices = (stackId: string, fn: (s: Service) => Service) => db.patchStack(stackId, (s) => ({ ...s, services: s.services.map(fn) }));

  const adapter: DockletAdapter = {
    kind: "demo",
    ...createDemoRegistries(() => db.changed("registries")),
    getAuthMode: async () => db.settings.auth.mode,
    getCurrentUser: async () => ({ id: "demo-admin", username: db.settings.auth.local.username, requiresPasswordChange: false }),
    login: async (username) => ({ id: "demo-admin", username, requiresPasswordChange: false }),
    logout: async () => undefined,
    changePassword: async () => undefined,
    updateMyLocale: async (locale) => ({ id: "demo-admin", username: db.settings.auth.local.username, locale, requiresPasswordChange: false }),
    ...createDemoHubBackups(options),
    getCapability: async (operationId, resourceRef = HUB_REF) => {
      const knownResource = resourceRef.kind === "hub" ||
        (resourceRef.kind === "host" && !!db.host(resourceRef.id)) ||
        (resourceRef.kind === "stack" && !!db.stack(resourceRef.id)) ||
        (resourceRef.kind === "container" && db.containers().some((item) => item.id === resourceRef.id)) ||
        (resourceRef.kind === "network" && db.topology.networks.some((item) => item.id === resourceRef.id)) ||
        (resourceRef.kind === "image" && (await adapter.listImages()).some((item) => item.id === resourceRef.id)) ||
        (resourceRef.kind === "registry" && (await adapter.listContainerRegistries()).some((item) => item.id === resourceRef.id));
      const available = knownResource && supportedOperations.has(operationId);
      return { operationId, resourceRef, available, ...(available ? {} : { disabledReasonCode: GENERIC_REASON }) };
    },
    ...createHostsApi(ctx),
    ...createImagesApi(ctx),
    ...createVolumesApi(ctx),
    ...createNetworksApi(ctx),
    ...createSettingsApi(ctx),
    ...createDemoNotifications(ctx),

    async listHosts() {
      const latency = simulationStore.get().latency;
      if (latency !== "instant") await new Promise((r) => setTimeout(r, latency === "short" ? 250 : 900));
      if (simulationStore.get().failArea === "hosts") throw new Error("Simulierter Fehler beim Laden (hosts)");
      return clone(db.hosts);
    },
    listStacks: () => simulateRead("stacks", () => (empty() ? [] : clone(withoutSystem(db.discoveredStacks())))),
    getStack: (id) =>
      simulateRead("stacks", (): CodedStackDetail => {
        const s = db.stack(id);
        if (!s) throw new DemoCommandError("not-found");
        return encodeDemoStackDetail(clone({
          ...s,
          compose: db.compose[id] ?? "",
          env: db.env[id] ?? "",
          deployments: db.deploymentsFor(id, runner.list()),
          restorePoints: db.restorePoints.filter((r) => r.stackId === id),
        }));
      }),
    listContainers: () => simulateRead("containers", () => (empty() ? [] : clone(db.containers()))),
    getSecurity: () => simulateRead("security", () => (empty() ? { ...clone(db.security), images: [], findings: [], history: [] } : clone(db.security))),
    listEvents: () => simulateRead("dashboard", () => (empty() ? [] : clone(db.events).map(encodeDemoEvent))),
    listSchedule: () => simulateRead("dashboard", () => (empty() ? [] : clone(db.schedule))),
    listActions: async () => clone(encodeDemoRuns(runner.list())),

    async runStackCommand(stackId, command: StackCommand) {
      if (!(await adapter.getCapability("runStackCommand", { kind: "stack", id: stackId })).available) {
        throw unavailable("runStackCommand", { kind: "stack", id: stackId });
      }
      const stack = requireStack(stackId);
      const names = stack.services.map((s) => s.name).join(", ");
      const fail = failStep(takeDemoFailure(adapter, "runStackCommand")?.at);
      const labels: Record<StackCommand, string> = { start: "Start", stop: "Stopp", restart: "Neustart", redeploy: "Pull & Redeploy" };
      const steps: Record<StackCommand, StepPlan[]> = {
        start: [
          { key: "validate", label: "Prüfung", detail: "Compose gültig" },
          { key: "start", label: "Start", detail: names },
          { key: "health", label: "Zustandsprüfung" },
        ],
        stop: [
          { key: "validate", label: "Prüfung" },
          { key: "stop", label: "Stopp", detail: names },
        ],
        restart: [
          { key: "stop", label: "Stopp", detail: names },
          { key: "start", label: "Start", detail: names },
          { key: "health", label: "Zustandsprüfung" },
        ],
        redeploy: [
          { key: "validate", label: "Prüfung", detail: "Compose gültig, Host erreichbar" },
          { key: "pull", label: "Image-Pull", detail: stack.services.map((s) => imageRef(s.image)).join(", ") },
          { key: "create", label: "Austausch", detail: `${stack.services.length} Container ersetzt` },
          { key: "start", label: "Start", detail: names },
          { key: "health", label: "Zustandsprüfung" },
        ],
      };
      const rp = command === "redeploy" ? db.saveRestorePoint(stack, `Vor Redeploy · ${stack.name}`) : undefined;
      return runForStack(stack, command === "redeploy" ? "redeploy" : command, `${labels[command]} · ${stack.name}`, steps[command], {
        failAt: fail,
        failMessage: fail ? "Simulierter Fehler (Design-Labor)" : undefined,
        restorePointId: rp?.id,
        onSuccess: () => {
          const now = new Date().toISOString();
          setServices(stackId, (s) => ({ ...s, state: command === "stop" ? "stopped" : "running", startedAt: command === "stop" ? undefined : now }));
          for (const s of stack.services) logs.append(stackId, s.name, "info", command === "stop" ? "Container gestoppt (SIGTERM)" : "Container gestartet");
          if (command === "redeploy") db.patchStack(stackId, (s) => ({ ...s, lastDeployAt: now }));
        },
        onFailure: (_run, step) => {
          if (step.key === "health") for (const s of stack.services.filter((x) => x.health === "unhealthy")) logs.append(stackId, s.name, "error", "healthcheck: exit 1 (timeout)");
        },
      });
    },

    async installStackUpdates(stackId) {
      const stack = requireStack(stackId);
      const pending = stack.services.filter((s) => hasUpdate(s.update));
      const rp = db.saveRestorePoint(stack, `Vor Update · ${stack.name}`);
      const fail = failStep(takeDemoFailure(adapter, "installStackUpdates")?.at);
      const latest = (s: Service) => (s.update.state === "version" || s.update.state === "rebuild" ? s.update.latest : s.image);
      return runForStack(
        stack,
        "update",
        `Update · ${stack.name}`,
        [
          { key: "validate", label: "Prüfung", detail: "Vorheriges Image und compose.yaml gesichert" },
          { key: "pull", label: "Image-Pull", detail: pending.map((s) => `${s.name} ${latest(s).version ?? latest(s).tag}`).join(", ") },
          { key: "create", label: "Austausch", detail: pending.map((s) => s.name).join(", ") },
          { key: "start", label: "Start" },
          { key: "health", label: "Zustandsprüfung" },
        ],
        {
          services: pending.map((s) => s.name),
          failAt: fail,
          failMessage: fail ? "Simulierter Fehler (Design-Labor)" : undefined,
          restorePointId: rp.id,
          onSuccess: () => applyUpdates(stackId, pending, false),
          onFailure: (_run, step) => {
            if (step.key === "start" || step.key === "health") applyUpdates(stackId, pending, true);
          },
        },
      );

      function applyUpdates(id: string, list: Service[], broken: boolean) {
        const names = new Set(list.map((s) => s.name));
        let compose = db.compose[id] ?? "";
        setServices(id, (s) => {
          if (!names.has(s.name)) return s;
          const next = latest(s);
          if (!sameImageReference(imageRef(s.image), imageRef(next))) compose = compose.replace(imageRef(s.image), imageRef(next));
          return {
            ...s,
            image: next,
            health: broken ? "unhealthy" : s.health === "unhealthy" ? "healthy" : s.health,
            startedAt: new Date().toISOString(),
            update: { state: "current", current: next, checkedAt: new Date().toISOString(), registry: s.update.state === "version" || s.update.state === "rebuild" ? s.update.registry : "Docker Hub · anonym" },
          };
        });
        db.compose[id] = compose;
        db.patchStack(id, (s) => ({ ...s, lastDeployAt: new Date().toISOString() }));
      }
    },

    async checkUpdates(scope) {
      const stack = scope.stackId ? requireStack(scope.stackId) : undefined;
      const container = scope.containerId ? db.containers().find((c) => c.id === scope.containerId) : undefined;
      const hostId = stack?.hostId ?? container?.hostId;
      const offline = hostId ? db.host(hostId)?.status === "offline" : false;
      return runner.start({
        kind: "check-updates",
        title: `Update-Prüfung · ${stack?.name ?? container?.name ?? "Alle Hosts"}`,
        target: stack ? hostLabel(stack) : (container?.name ?? "Alle Hosts"),
        hostId,
        stackId: stack?.id,
        containerId: container?.id,
        steps: [
          { key: "collect", label: "Registries abfragen", detail: "Simulation – keine Netzwerkanfrage", ms: 1100 },
          { key: "validate", label: "Versionen vergleichen" },
          { key: "save", label: "Ergebnis speichern", detail: "Modus „nur prüfen“ – nichts installiert" },
        ],
        failAt: offline ? "collect" : undefined,
        failMessage: offline ? "Host ist nicht verbunden" : undefined,
        onSuccess: (run) => {
          const at = new Date().toISOString();
          const touch = <T extends { update: Service["update"] }>(x: T): T => ("checkedAt" in x.update ? { ...x, update: { ...x.update, checkedAt: at } } : x);
          if (stack) setServices(stack.id, touch);
          else if (container?.stackId && container.serviceName) db.patchService(container.stackId, container.serviceName, touch);
          else if (container) db.patchContainer(container.id, touch);
          else db.stacks = db.stacks.map((s) => (db.host(s.hostId)?.status === "offline" ? s : { ...s, services: s.services.map(touch) }));
          db.pushEvent({ kind: "update", level: "info", title: `${run.title}: abgeschlossen`, hostId, stackId: stack?.id, actionId: run.id });
          db.changed("stacks", "containers");
        },
      });
    },

    async deployStack(input: DeployInput) {
      const fail = failStep(takeDemoFailure(adapter, "deployStack")?.at);
      let stack = input.stackId ? db.stack(input.stackId) : undefined;
      if (!stack) {
        const base = (input.name || "stack").toLowerCase().replace(/[^a-z0-9-]+/g, "-");
        let id = base;
        let n = 2;
        while (db.stack(id)) id = `${base}-${n++}`;
        stack = { id, name: input.name || id, hostId: input.hostId, labels: input.labels, autoUpdate: { mode: "check" }, services: [] };
        db.stacks = [...db.stacks, stack];
      } else if (stack.services.length) {
        db.saveRestorePoint(stack, `Vor Deploy · ${stack.name}`);
      }
      const id = stack.id;
      db.compose[id] = input.compose;
      db.env[id] = input.env;
      db.patchStack(id, (s) => ({ ...s, labels: input.labels, hostId: input.hostId }));
      const fresh = requireStack(id);
      const actionId = runForStack(
        fresh,
        input.stackId ? "redeploy" : "deploy",
        `${input.stackId ? "Speichern & Redeploy" : "Deploy"} · ${fresh.name}`,
        [
          { key: "validate", label: "Prüfung", detail: "Compose gültig" },
          { key: "pull", label: "Image-Pull" },
          { key: "create", label: input.stackId ? "Austausch" : "Erstellung" },
          { key: "start", label: "Start" },
          { key: "health", label: "Zustandsprüfung" },
        ],
        {
          failAt: fail,
          failMessage: fail ? "Simulierter Fehler (Design-Labor)" : undefined,
          output: composeScript({ project: id, services: draftServices(input.compose), hostName: db.host(input.hostId)?.name ?? input.hostId }),
          onSuccess: () => {
            db.syncServicesFromCompose(id);
            syncStackNetworks(db, id);
          },
        },
      );
      db.changed("stacks");
      return { actionId, stackId: id };
    },

    async saveStackFiles(stackId, compose, env) {
      const stack = requireStack(stackId);
      db.compose[stackId] = compose;
      db.env[stackId] = env;
      db.pushEvent({ kind: "config", level: "info", title: `compose.yaml von ${stack.name} gespeichert`, hostId: stack.hostId, stackId });
      db.changed("stacks");
    },

    async restoreRestorePoint(stackId, restorePointId) {
      const stack = requireStack(stackId);
      const rp = db.restorePoints.find((r) => r.id === restorePointId);
      if (!rp) throw new DemoCommandError("not-found");
      return runForStack(
        stack,
        "restore",
        `Wiederherstellung · ${stack.name}`,
        [
          { key: "validate", label: "Prüfung", detail: `Stand „${rp.reason}“ vorhanden` },
          { key: "create", label: "Vorherigen Stand einspielen", detail: rp.images.map((i) => `${i.service} ${i.image.version ?? i.image.tag}`).join(", ") },
          { key: "start", label: "Start" },
          { key: "health", label: "Zustandsprüfung" },
        ],
        {
          services: rp.images.map((i) => i.service),
          onSuccess: () => {
            db.compose[stackId] = rp.composeSnapshot;
            syncStackNetworks(db, stackId);
            setServices(stackId, (s) => {
              const old = rp.images.find((i) => i.service === s.name)?.image;
              if (!old || sameImageReference(imageRef(old), imageRef(s.image))) return s;
              return {
                ...s,
                image: old,
                health: s.health === "unhealthy" ? "healthy" : s.health,
                update: { state: "version", current: old, latest: s.image, level: "minor", checkedAt: new Date().toISOString(), registry: "Docker Hub · anonym" },
              };
            });
          },
        },
      );
    },

    async setStackLabels(stackId, labels) {
      db.patchStack(stackId, (s) => ({ ...s, labels: normalizeStackLabels(labels) }));
      db.changed("stacks");
    },

    async setAutoUpdate(stackId, policy) {
      db.patchStack(stackId, (s) => ({ ...s, autoUpdate: policy }));
      db.changed("stacks", "schedule");
    },

    async restartService(stackId, service) {
      const stack = requireStack(stackId);
      const svc = stack.services.find((s) => s.name === service);
      const unhealthy = svc?.health === "unhealthy";
      return runForStack(
        stack,
        "restart",
        `Neustart · ${service}`,
        [
          { key: "stop", label: "Stopp", detail: service },
          { key: "start", label: "Start", detail: service },
          { key: "health", label: "Zustandsprüfung" },
        ],
        {
          services: [service],
          failAt: unhealthy ? "health" : undefined,
          failMessage: unhealthy ? `${service} bleibt unhealthy – Logs prüfen oder vorherigen Stand wiederherstellen` : undefined,
          onSuccess: () => {
            db.patchService(stackId, service, (s) => ({ ...s, state: "running", startedAt: new Date().toISOString() }));
            logs.append(stackId, service, "info", "Container neu gestartet (manuell)");
          },
          onFailure: () => logs.append(stackId, service, "error", "healthcheck: exit 1 nach Neustart"),
        },
      );
    },

    async convertDockerRun(_hostId, command) {
      const conv = convertDockerRun(command, []);
      return { compose: stringify({ services: { [conv.key]: conv.body } }, { lineWidth: 0 }), env: "", serviceName: conv.key };
    },

    async importSource(request) {
      await new Promise((r) => setTimeout(r, 700));
      return simulateImport(request);
    },

    async runContainerCommand(id, command) {
      const c = findContainer(id);
      const label = { start: "Start", stop: "Stopp", restart: "Neustart" }[command];
      const steps: StepPlan[] =
        command === "start" ? [{ key: "start", label: "Start" }] : command === "stop" ? [{ key: "stop", label: "Stopp" }] : [{ key: "stop", label: "Stopp" }, { key: "start", label: "Start" }];
      return runner.start({
        kind: command,
        title: `${label} · ${c.name}`,
        target: `${c.name} · ${db.host(c.hostId)?.name ?? ""}`,
        hostId: c.hostId,
        containerId: c.id,
        services: [c.name],
        steps,
        output: containerScript(c.name, imageRef(c.image)),
        failAt: db.host(c.hostId)?.status === "offline" ? steps[0]?.key : undefined,
        failMessage: "Host ist nicht verbunden",
        onSuccess: () => {
          const state = command === "stop" ? "stopped" : "running";
          if (c.stackId && c.serviceName) db.patchService(c.stackId, c.serviceName, (s) => ({ ...s, state }));
          else db.patchContainer(c.id, (x) => ({ ...x, state }));
          db.changed("containers", "stacks");
        },
      });
    },

    async recreateContainer(id, input) {
      const c = findContainer(id);
      if (c.origin !== "standalone") throw new DemoCommandError("recreate-not-allowed");
      const [repository = c.image.repository, tag = "latest"] = input.image.split(/:(?=[^:/]+$)/);
      const fail = failStep(takeDemoFailure(adapter, "recreateContainer")?.at);
      return runner.start({
        kind: "recreate",
        title: `Neu erstellen · ${c.name}`,
        target: `${c.name} · ${db.host(c.hostId)?.name ?? ""}`,
        hostId: c.hostId,
        containerId: c.id,
        services: [c.name],
        steps: [
          { key: "validate", label: "Prüfung", detail: "Konfiguration gültig, vorherige gesichert" },
          { key: "pull", label: "Image-Pull", detail: input.image },
          { key: "create", label: "Neu erstellen" },
          { key: "start", label: "Start" },
          { key: "health", label: "Zustandsprüfung" },
        ],
        output: containerScript(c.name, input.image),
        failAt: fail,
        failMessage: fail ? "Simulierter Fehler (Design-Labor)" : undefined,
        onSuccess: () => {
          db.patchContainer(c.id, (x) => {
            const image = { repository, tag, version: tag.replace(/^v/, ""), published: new Date().toISOString() };
            return { ...x, image, ports: input.ports, env: input.env, restartPolicy: input.restartPolicy, state: "running", startedAt: new Date().toISOString(), update: { state: "current", current: image, checkedAt: new Date().toISOString(), registry: "Docker Hub · anonym" } };
          });
          db.pushEvent({ kind: "deploy", level: "ok", title: `${c.name} neu erstellt`, hostId: c.hostId });
          db.changed("containers");
        },
        onFailure: () => db.pushEvent({ kind: "deploy", level: "error", title: `${c.name}: Neu erstellen fehlgeschlagen`, hostId: c.hostId }),
      });
    },

    async setContainerOrigin(id, origin) {
      db.patchContainer(id, (c) => ({ ...c, origin, originNote: "Manuell zugeordnet" }));
      db.changed("containers");
    },

    async installContainerUpdate(id) {
      const c = findContainer(id);
      if (c.stackId && c.serviceName) return adapter.installStackUpdates(c.stackId);
      if (c.origin === "unraid") throw new DemoCommandError("managed-by-unraid");
      if (c.origin !== "standalone") throw new DemoCommandError("recreate-not-allowed");
      const next = c.update.state === "version" || c.update.state === "rebuild" ? c.update.latest : c.image;
      return adapter.recreateContainer(id, { image: imageRef(next), ports: c.ports, env: c.env, restartPolicy: c.restartPolicy });
    },

    async reconnectHost(hostId) {
      const host = db.host(hostId);
      return runner.start({
        kind: "reconnect",
        title: `Verbindung prüfen · ${host?.name ?? hostId}`,
        target: host?.name ?? hostId,
        hostId,
        steps: [
          { key: "validate", label: "Hub-Endpunkt prüfen", detail: host?.address },
          { key: "collect", label: "Agent-Status abfragen" },
        ],
        failAt: host?.status === "offline" ? "collect" : undefined,
        failMessage: "Edge-Agent meldet sich nicht (Simulation). Agent auf dem Host prüfen.",
        onFailure: () => db.pushEvent({ kind: "host", level: "warn", title: `${host?.name}: weiterhin nicht verbunden`, hostId }),
      });
    },

    async runScan() {
      const images = db.security.images.filter((i) => db.host(i.hostId)?.status !== "offline");
      return runner.start({
        kind: "scan",
        title: "Schwachstellenscan",
        target: "Alle verbundenen Hosts",
        steps: [
          { key: "collect", label: "Images ermitteln", detail: `${images.length} Images` },
          { key: "pull", label: "Datenbank aktualisieren", detail: "Simulation – keine Netzwerkanfrage" },
          { key: "scan", label: "Images prüfen", detail: `${images.length} Images`, ms: 2200 },
          { key: "save", label: "Ergebnisse speichern" },
        ],
        onSuccess: () => {
          const at = new Date().toISOString();
          const enabledBefore = db.security.enabled;
          db.security.enabled = true;
          db.security.images = db.security.images.map((i) => ({ ...i, scannedAt: at }));
          const counts = { critical: 0, high: 0, medium: 0, low: 0 };
          for (const i of db.security.images) for (const k of Object.keys(counts) as (keyof typeof counts)[]) counts[k] += i.counts[k];
          db.security.history = [...db.security.history, { at, trigger: "manual" as const, counts }].slice(-14);
          db.pushEvent({ kind: "scan", level: "info", title: `Schwachstellenscan abgeschlossen (${images.length} Images)` });
          // A manual scan switches scans on, which the settings show too.
          if (enabledBefore) db.changed("security");
          else db.changed("security", "settings");
        },
      });
    },

    async setScanEnabled(enabled) {
      await simulateWrite("security");
      db.security.enabled = enabled;
      db.changed("security", "settings");
    },

    subscribeActions: (listener) => runner.subscribe((runs) => listener(encodeDemoRuns(runs))),
    subscribeMetrics: (listener) => metrics.subscribe(listener),
    subscribeLogs: (stackId, listener) => logs.subscribe(stackId, listener),
    subscribeContainerLogs: (containerId, listener) => {
      const c = db.containers().find((x) => x.id === containerId);
      if (!c) {
        listener([]);
        return () => undefined;
      }
      if (c.stackId && c.serviceName) {
        const service = c.serviceName;
        return logs.subscribe(c.stackId, (lines) => listener(lines.filter((l) => l.service === service)));
      }
      return logs.subscribe(`container:${c.id}`, listener);
    },
    subscribeChanges: (listener) => db.subscribe(listener),
  };

  function findContainer(id: string): ContainerInfo {
    const c = db.containers().find((x) => x.id === id);
    if (!c) throw new DemoCommandError("not-found");
    return c;
  }

  for (const [operationId, value] of Object.entries(adapter)) {
    if (operationId !== "getCapability" && typeof value === "function") supportedOperations.add(operationId);
  }
  supportedOperations.add("clearContainerRegistryCredential");
  for (const operationId of options.missingCapabilities ?? []) supportedOperations.delete(operationId);
  registerDemoOptions(adapter, options);
  return adapter;
}

/** Services of a submitted draft, for the simulated Compose output of its first deploy. */
function draftServices(compose: string): { name: string; image: string }[] {
  const parsed = parseCompose(compose);
  return parsed.ok ? parsed.model.services.map((m) => ({ name: m.key, image: m.image })) : [];
}

export { containerId };
