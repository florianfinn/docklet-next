// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original dev-only origin, recreate identity and compact result simulation.

import { intentFingerprint } from "../intent-key";
import type { DockletAdapter, RecreateInput } from "../adapter";
import { AdapterOutcomeError, type ResourceRef } from "../capability";
import type { ActionRun, ContainerInfo, DeployStepKey } from "@/domain/model";
import { imageRef } from "@/domain/updates";
import { isSystemWorkload } from "@/domain/system";
import type { DemoContext } from "./demo-context";
import type { FailAt } from "./options";
import { containerScript } from "./run-output";
import { encodeDemoRun } from "./encode-message";

const problem = (code: string) => new AdapterOutcomeError({ kind: "error", code, status: 409 });
const hasCompose = (c: ContainerInfo) => Object.keys(c.dockerLabels ?? {}).some((key) => key.startsWith("com.docker.compose."));

export function createDemoContainerOperations({ db, runner, demoFailure }: DemoContext, failStep: (at: FailAt | undefined) => DeployStepKey | undefined) {
  const detected = new Map(db.standalone.map((c) => [c.id, c.origin]));
  const receipts = new Map<string, { digest: string; run: ActionRun }>();
  const find = (id: string) => {
    const c = db.containers().find((item) => item.id === id);
    if (!c) throw problem("runtime.target-removed");
    return c;
  };
  const capabilityReason = (operation: string, ref: ResourceRef): string | undefined => {
    if (ref.kind !== "container" || !["recreateContainer", "setContainerOrigin"].includes(operation)) return undefined;
    const c = db.containers().find((item) => item.id === ref.id);
    if (!c) return "capability.unavailable";
    if (operation === "setContainerOrigin") return detected.get(c.id) === "unknown" || c.originSource === "unconfirmed" ? "" : "container.origin-locked";
    return c.origin === "standalone" && c.originSource !== "unconfirmed" && !hasCompose(c) && !isSystemWorkload(c) && !c.restrictions?.recreate ? "" : "container.origin-locked";
  };
  const api: Pick<DockletAdapter, "recreateContainer" | "setContainerOrigin" | "getContainerEditConfig" | "getRuntimeRun" | "observeActionDetail"> = {
    async getContainerEditConfig(id) {
      const c = find(id);
      if (capabilityReason("recreateContainer", { kind: "container", id })) throw problem("container.origin-locked");
      return { id, name: c.name, image: imageRef(c.image), running: c.state === "running", environment: Object.entries(c.env).map(([key, value]) => `${key}=${value}`),
        hostConfig: { restartPolicy: { name: c.restartPolicy }, portBindings: Object.fromEntries(c.ports.map((p) => [`${p.container}/${p.protocol ?? "tcp"}`, p.host ? [{ hostPort: p.host }] : []])) } };
    },
    async setContainerOrigin(id, origin) {
      const c = find(id);
      if (capabilityReason("setContainerOrigin", { kind: "container", id }) || origin === "compose" || (origin === "standalone" && hasCompose(c))) throw problem("container.origin-locked");
      db.patchContainer(id, (item) => ({ ...item, origin: origin === "unknown" ? detected.get(id) ?? "unknown" : origin, originSource: origin === "unknown" ? "detected" : "manual", originNote: undefined }));
      db.changed("containers", "stacks");
    },
    async recreateContainer(id, input: RecreateInput, key) {
      const digest = intentFingerprint(input);
      const scope = key ? `${id}:${key}` : undefined;
      const prior = scope ? receipts.get(scope) : undefined;
      if (prior) { if (prior.digest !== digest) throw problem("runtime.idempotency-mismatch"); return prior.run.id; }
      const c = find(id);
      if (capabilityReason("recreateContainer", { kind: "container", id })) throw problem("container.origin-locked");
      const imageText = input.image ?? imageRef(c.image);
      const [repository = c.image.repository, tag = "latest"] = imageText.split(/:(?=[^:/]+$)/);
      const replacement = db.nextId("container");
      const result = { oldId: id, newId: replacement, oldName: c.name, newName: input.name ?? c.name };
      const fail = failStep(demoFailure("recreateContainer")?.at);
      return runner.start({ kind: "recreate", title: `Neu erstellen · ${c.name}`, target: `${c.name} · ${db.host(c.hostId)?.name ?? ""}`, hostId: c.hostId, containerId: id, services: [c.name],
        steps: [{ key: "validate", label: "Prüfung", detail: "Konfiguration gültig, vorherige gesichert" }, { key: "pull", label: "Image-Pull", detail: imageText }, { key: "create", label: "Neu erstellen" }, { key: "start", label: "Start" }, { key: "health", label: "Zustandsprüfung" }],
        recreateResult: result, output: containerScript(c.name, imageText), failAt: fail, failMessage: fail ? "container.recreate-recovered" : undefined,
        onStart: (run) => { if (scope) receipts.set(scope, { digest, run }); },
        onSuccess: (run) => {
          db.patchContainer(id, (item) => {
            const image = { repository, tag, version: tag.replace(/^v/, ""), published: new Date().toISOString() };
            const ports = input.hostConfig?.portBindings;
            return { ...item, id: replacement, name: result.newName, image,
              ports: ports ? Object.entries(ports).flatMap(([port, bindings]) => (bindings ?? []).map((binding) => ({ container: port.split("/")[0]!, host: binding.hostPort, protocol: port.endsWith("/udp") ? "udp" as const : "tcp" as const }))) : item.ports,
              env: input.environment ? Object.fromEntries(input.environment.map((entry) => { const at = entry.indexOf("="); return [entry.slice(0, at), entry.slice(at + 1)]; })) : item.env,
              restartPolicy: input.hostConfig?.restartPolicy?.name ?? item.restartPolicy, state: c.state,
              update: { state: "current", current: image, checkedAt: new Date().toISOString(), registry: "Docker Hub · anonym" } };
          });
          db.topology = { ...db.topology, attachments: db.topology.attachments.map((attachment) => attachment.containerId === id ? { ...attachment, containerId: replacement } : attachment) };
          if (db.isolated.delete(id)) db.isolated.add(replacement);
          detected.set(replacement, detected.get(id) ?? c.origin); detected.delete(id);
          if (scope) receipts.set(scope, { digest, run });
          db.pushEvent({ kind: "deploy", level: "ok", title: `${c.name} neu erstellt`, hostId: c.hostId }); db.changed("containers", "topology");
        },
        onFailure: (run) => { if (scope) receipts.set(scope, { digest, run }); db.pushEvent({ kind: "deploy", level: "error", title: `${c.name}: Neu erstellen fehlgeschlagen`, hostId: c.hostId }); },
      });
    },
    async getRuntimeRun(host, id) {
      const run = runner.list().find((item) => item.id === id) ?? [...receipts.values()].find((receipt) => receipt.run.id === id)?.run;
      if (!run || run.hostId !== host) throw problem("runtime.target-removed");
      return { ...encodeDemoRun(run), outcomeState: "known", cancelRequested: false };
    },
    observeActionDetail(host, id, listener) {
      return runner.subscribe((runs) => {
        const run = runs.find((item) => item.id === id) ?? [...receipts.values()].find((receipt) => receipt.run.id === id)?.run;
        if (run && (run.hostId ?? "0") === host) listener({ status: "ready", run: encodeDemoRun(run) });
        else listener({ status: "error", error: problem("runtime.target-removed") });
      });
    },
  };
  return { api, capabilityReason };
}
