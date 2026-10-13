import { AGENT_IMAGE } from "@/domain/agent-install";
import type { AgentReport, ContainerInfo, FileOwner, Host, ImageVersion, Stack } from "@/domain/model";
import { AGENT_LABELS, type SystemComponent } from "@/domain/system";
import { isInside } from "@/domain/validation";

import { daysAgo } from "./time";

const COMPOSE_PROJECT = "docklet-agent";
const LABELS: Record<string, string> = { ...AGENT_LABELS, "com.docker.compose.project": COMPOSE_PROJECT, "com.docker.compose.service": "agent" };

export function agentImage(version: string): ImageVersion {
  const [repository = AGENT_IMAGE, tag = version] = AGENT_IMAGE.split(/:(?=[^:]+$)/);
  return { repository, tag: version || tag, version: version || tag, published: daysAgo(12) };
}

/** Docklet's agent as Docker reports it; the label, not the name, marks it as a system workload. */
export function agentContainer(host: Host): ContainerInfo {
  const image = agentImage(host.agentVersion !== "–" ? host.agentVersion : "0.33.0");
  return {
    id: `sys-agent-${host.id}`,
    name: "docklet-agent",
    hostId: host.id,
    stackId: undefined,
    serviceName: "agent",
    origin: "compose",
    originNote: `Compose-Projekt ${COMPOSE_PROJECT} (Systemkomponente)`,
    image,
    state: host.status === "online" ? "running" : "unknown",
    health: "none",
    ports: host.connection === "direct" ? [{ host: "3552", container: "3552" }] : [],
    networks: ["docklet-agent_default"],
    mounts: ["/var/run/docker.sock:/var/run/docker.sock", `${host.projectDir}:${host.projectDir}`, `${host.installDir}/data:/data`],
    env: {},
    restartPolicy: "unless-stopped",
    memLimitMb: 256,
    startedAt: host.lastSeen,
    update: { state: "current", current: image, checkedAt: host.lastSeen, registry: "Docklet-Registry (Beispiel)" },
    dockerLabels: { ...LABELS },
  };
}

/** Compose project found by stack discovery when the agent folder sits in the project directory. */
export function agentStack(host: Host): Stack {
  const c = agentContainer(host);
  return {
    id: `sys-agent-${host.id}`,
    name: COMPOSE_PROJECT,
    hostId: host.id,
    description: "Docklet-Agent",
    labels: [],
    services: [
      {
        name: "agent",
        image: c.image,
        state: c.state,
        health: "none",
        ports: c.ports,
        networks: c.networks,
        restartPolicy: c.restartPolicy,
        memLimitMb: c.memLimitMb,
        startedAt: c.startedAt,
        restarts24h: 0,
        update: c.update,
      },
    ],
    autoUpdate: { mode: "check" },
    dockerLabels: { ...LABELS },
  };
}

export function agentInsideProject(host: Host): boolean {
  return isInside(host.installDir, host.projectDir);
}

export function systemComponent(host: Host): SystemComponent {
  const c = agentContainer(host);
  return {
    hostId: host.id,
    role: "agent",
    containerName: c.name,
    composeProject: COMPOSE_PROJECT,
    image: c.image,
    state: c.state,
    installDir: host.installDir,
    insideProjectDir: agentInsideProject(host),
    labels: { ...LABELS },
    startedAt: c.startedAt,
    ports: c.ports,
    mounts: c.mounts,
    networks: c.networks,
    restartPolicy: c.restartPolicy,
    memLimitMb: c.memLimitMb,
    recentLog: agentLog(host),
  };
}

/** Synthetic agent log: connection, inventory and heartbeat lines; ends at the last contact. */
function agentLog(host: Host): SystemComponent["recentLog"] {
  const last = new Date(host.lastSeen).getTime();
  const at = (minutesBefore: number) => new Date(last - minutesBefore * 60_000).toISOString();
  const lines: SystemComponent["recentLog"] = [
    { at: at(42), level: "info", message: host.connection === "direct" ? "listening on :3552, waiting for hub (Simulation)" : "connecting to hub (outbound, Simulation)" },
    { at: at(41), level: "info", message: "hub session established, token accepted" },
    { at: at(40), level: "info", message: `inventory sent: project directory ${host.projectDir}` },
    { at: at(12), level: "debug", message: "docker events stream resumed" },
    { at: at(1), level: "info", message: "heartbeat ok" },
  ];
  if (host.status === "offline") lines.push({ at: at(0), level: "warn", message: "hub unreachable, retrying with backoff (letzter Stand vor der Trennung)" });
  return lines;
}

/** Owner the simulated installer reads locally on the host; mirrors common layouts. */
export function localDirectoryOwner(path: string): { owner: FileOwner; name: string; note: AgentReport["note"]; socketGid: number } {
  if (path.startsWith("/mnt/user")) return { owner: { uid: 99, gid: 100 }, name: "nobody:users", note: "unraid", socketGid: 281 };
  if (path === "/opt" || path === "/srv" || path.startsWith("/opt/")) return { owner: { uid: 0, gid: 0 }, name: "root:root", note: "root-owned", socketGid: 998 };
  if (path.startsWith("/srv/")) return { owner: { uid: 1001, gid: 1001 }, name: "deploy:deploy", note: "ok", socketGid: 998 };
  return { owner: { uid: 1000, gid: 1000 }, name: "docker:docker", note: "ok", socketGid: 998 };
}

/** Report the agent sends after connecting: what the installer found locally, and which IDs it used. */
export function agentReport(host: Pick<Host, "projectDir" | "fileOwner" | "socketGid">, reportedAt: string, socketManual = false): AgentReport {
  const local = localDirectoryOwner(host.projectDir);
  return {
    reportedAt,
    projectDirOwner: { ...local.owner, name: local.name },
    projectDirWritable: true,
    fileOwner: host.fileOwner ?? local.owner,
    fileOwnerSource: host.fileOwner ? "manual" : "directory-owner",
    socketGid: host.socketGid ?? local.socketGid,
    socketGidSource: socketManual ? "manual" : "detected",
    note: local.note,
    simulated: true,
  };
}
