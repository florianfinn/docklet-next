import type { Host } from "@/domain/model";
import { agentHubUrl } from "@/domain/settings";
import { slugify } from "@/domain/validation";

import type { HostsApi } from "../api";
import type { StepPlan } from "./action-runner";
import { DemoError, failMessage, wait, type DemoContext } from "./demo-context";
import { agentReport, systemComponent } from "./fixtures/system";
import { containerScript } from "./run-output";

const TOKEN_CHARS = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";

function randomBlock(n: number): string {
  let out = "";
  for (let i = 0; i < n; i++) out += TOKEN_CHARS[Math.floor(Math.random() * TOKEN_CHARS.length)];
  return out;
}

function demoAgentToken(name: string): string {
  return `DEMO-NUR-ANSICHT-${slugify(name).slice(0, 12).toUpperCase()}-${randomBlock(4)}-${randomBlock(4)}`;
}

export function createHostsApi({ db, runner, forcedFailure, demoFailure }: DemoContext): HostsApi {
  /** Edge agents dial the internal hub address; nothing is exposed to the internet. */
  const hubHost = () => {
    const url = agentHubUrl(db.settings.general);
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  };
  const manualSocket = new Set<string>();

  return {
    async registerHost(input) {
      await wait(350);
      if (db.hosts.some((h) => h.name.toLowerCase() === input.name.trim().toLowerCase())) throw new DemoError("exists");
      const base = slugify(input.name);
      let id = base;
      for (let n = 2; db.host(id); n++) id = `${base}-${n}`;
      const host: Host = {
        id,
        name: input.name.trim(),
        color: input.color,
        status: "pending",
        connection: input.connection,
        os: "–",
        dockerVersion: "–",
        agentVersion: "–",
        address: input.connection === "direct" ? input.address : `Edge über ${hubHost()}`,
        projectDir: input.projectDir,
        installDir: input.installDir,
        fileOwner: input.fileOwner,
        socketGid: input.socketGid,
        demoAdded: true,
        cpuCores: 4,
        memTotalGb: 16,
        diskTotalGb: 1000,
        lastSeen: new Date().toISOString(),
      };
      db.hosts = [...db.hosts, host];
      if (input.socketGid !== undefined) manualSocket.add(id);
      db.pushEvent({ kind: "host", level: "info", title: `${host.name} angelegt – wartet auf den Agent`, hostId: id });
      db.changed("hosts");
      return { host: structuredClone(host), token: demoAgentToken(input.name) };
    },

    async connectHost(hostId) {
      const host = db.host(hostId);
      if (!host) throw new DemoError("not-found");
      const running = runner.list().find((r) => r.hostId === hostId && r.kind === "onboard" && r.status === "running");
      if (running) return running.id;
      const direct = host.connection === "direct";
      const steps: StepPlan[] = direct
        ? [
            { key: "validate", label: "Adresse prüfen", detail: host.address },
            { key: "connect", label: "Hub verbindet sich mit dem Agent", detail: "Simulation – keine Netzwerkverbindung" },
            { key: "health", label: "Agent meldet lokale Prüfung", detail: "Verzeichnisbesitzer, Schreibrechte, Socket-Gruppe" },
          ]
        : [
            { key: "wait", label: "Auf ausgehende Agent-Verbindung warten", detail: `Agent → ${hubHost()}`, ms: 1600 },
            { key: "connect", label: "Agent authentifiziert", detail: "Demo-Token – Simulation" },
            { key: "health", label: "Agent meldet lokale Prüfung", detail: "Verzeichnisbesitzer, Schreibrechte, Socket-Gruppe" },
          ];
      const requested = !!demoFailure("connectHost");
      const fail = requested || forcedFailure();
      const failAt = fail ? (direct ? "connect" : "wait") : undefined;
      return runner.start({
        kind: "onboard",
        title: `Host verbinden · ${host.name}`,
        target: host.name,
        hostId,
        steps,
        failAt,
        failMessage: requested
          ? direct
            ? `${host.address} antwortet nicht (Simulation: Port gesperrt oder Agent nicht gestartet)`
            : `Keine Verbindung vom Agent (Simulation: interne Hub-Adresse ${hubHost()} vom Host aus nicht erreichbar)`
          : failMessage,
        onSuccess: () => {
          const now = new Date().toISOString();
          const report = agentReport(host, now, manualSocket.has(hostId));
          db.hosts = db.hosts.map((h) =>
            h.id === hostId ? { ...h, status: "online", os: "Debian 13", dockerVersion: "28.1", agentVersion: "0.33.0", socketGid: report.socketGid, agentReport: report, lastSeen: now } : h,
          );
          db.pushEvent({ kind: "host", level: "ok", title: `${host.name} verbunden (${direct ? "direkt" : "Edge"})`, hostId });
          db.changed("hosts", "topology", "volumes");
        },
        onFailure: () => {
          db.pushEvent({ kind: "host", level: "warn", title: `${host.name}: Agent nicht verbunden`, hostId });
          db.changed("hosts");
        },
      });
    },

    async removeHost(hostId) {
      await wait(250);
      const host = db.host(hostId);
      if (!host) throw new DemoError("not-found");
      if (db.stacks.some((s) => s.hostId === hostId) || db.standalone.some((c) => c.hostId === hostId)) throw new DemoError("host-in-use");
      db.hosts = db.hosts.filter((h) => h.id !== hostId);
      for (const [id, v] of db.createdVolumes) if (v.hostId === hostId) db.createdVolumes.delete(id);
      db.pushEvent({ kind: "host", level: "info", title: `${host.name} entfernt` });
      db.changed("hosts", "volumes");
    },

    async updateHost(hostId, input) {
      await wait(250);
      const host = db.host(hostId);
      if (!host) throw new DemoError("not-found");
      const name = input.name.trim();
      if (!name || name.length > 40) throw new DemoError("invalid-name");
      if (db.hosts.some((h) => h.id !== hostId && h.name.toLowerCase() === name.toLowerCase())) throw new DemoError("exists");
      if (!/^#[0-9a-f]{6}$/i.test(input.color)) throw new DemoError("invalid-color");
      const next: Host = { ...host, name, color: input.color.toLowerCase() };
      db.hosts = db.hosts.map((h) => (h.id === hostId ? next : h));
      if (host.name !== name) db.pushEvent({ kind: "config", level: "info", title: `Host ${host.name} heißt jetzt ${name}`, hostId });
      db.changed("hosts", "stacks", "containers", "topology", "volumes", "images");
      return structuredClone(next);
    },

    async restartAgent(hostId) {
      const host = db.host(hostId);
      if (!host) throw new DemoError("not-found");
      const component = systemComponent(host);
      const offline = host.status !== "online";
      return runner.start({
        kind: "agent-restart",
        title: `Agent neu starten · ${host.name}`,
        target: `${component.containerName} · ${host.name}`,
        hostId,
        services: [component.containerName],
        steps: [
          { key: "stop", label: "Agent stoppen", detail: component.containerName },
          { key: "start", label: "Agent starten" },
          { key: "connect", label: "Agent meldet sich zurück", detail: host.connection === "direct" ? "Hub verbindet sich zum Agent" : "Agent verbindet sich zum Hub" },
        ],
        output: containerScript(component.containerName, `${component.image.repository}:${component.image.tag}`),
        failAt: offline ? "stop" : undefined,
        failMessage: offline ? `${host.name} ist nicht verbunden – der Agent lässt sich nur über eine bestehende Verbindung neu starten.` : undefined,
        onSuccess: () => {
          const now = new Date().toISOString();
          db.hosts = db.hosts.map((h) => (h.id === hostId ? { ...h, lastSeen: now } : h));
          db.pushEvent({ kind: "host", level: "ok", title: `${host.name}: Agent neu gestartet`, hostId });
          db.changed("hosts");
        },
      });
    },

    async listSystemComponents() {
      await wait(120);
      return db.hosts.filter((h) => h.status !== "pending").map(systemComponent);
    },

    async issueAgentToken(hostId) {
      const hostName = db.host(hostId)?.name ?? hostId;
      await wait(150);
      return demoAgentToken(hostName);
    },
  };
}
