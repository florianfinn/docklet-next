// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original validated read-only existing-host installation plan with URL/IPv6 parsing. M14 host maintenance and availability.

import { AGENT_IMAGE, AGENT_PORT, buildInstallArtifacts, type AgentInstallPlan } from "@/domain/agent-install";
import type { Host } from "@/domain/model";
import type { GeneralSettings, HostDefaults } from "@/domain/settings";
import { agentHubUrl } from "@/domain/settings";
export const TOKEN_PLACEHOLDER = "example";
export function existingHostDefaults(host: Host, defaults: HostDefaults): HostDefaults {
  return { ...defaults, projectDir: host.projectDir || defaults.projectDir, installDir: host.installDir || defaults.installDir, puid: host.fileOwner ? String(host.fileOwner.uid) : defaults.puid, pgid: host.fileOwner ? String(host.fileOwner.gid) : defaults.pgid };
}
export function existingHostPlan(host: Host, defaults: HostDefaults, general: GeneralSettings, token = TOKEN_PLACEHOLDER, image?: string): AgentInstallPlan {
  let address = "", port = AGENT_PORT;
  if (host.connection === "direct") {
    const url = new URL(host.address.includes("://") ? host.address : `http://${host.address}`);
    if (!["http:", "https:"].includes(url.protocol) || !url.hostname || url.username || url.password || url.search || url.hash || !["", "/"].includes(url.pathname)) throw new Error("hosts.invalid-address");
    address = url.hostname; port = Number(url.port || (url.protocol === "https:" ? 443 : 80));
  }
  if ((defaults.puid === "") !== (defaults.pgid === "") || [defaults.puid, defaults.pgid].some(value => value !== "" && !/^\d+$/.test(value))) throw new Error("hosts.invalid-ids");
  const plan: AgentInstallPlan = {
    hostName: host.name, connection: host.connection, address, port,
    projectDir: defaults.projectDir, installDir: defaults.installDir,
    hubUrl: agentHubUrl(general), timeZone: general.timeZone, token, image: image ?? AGENT_IMAGE,
    ...(defaults.puid && defaults.pgid ? { fileOwner: { uid: Number(defaults.puid), gid: Number(defaults.pgid) } } : {}),
    ...(!defaults.detectSocketGroup && host.socketGid !== undefined ? { socketGid: host.socketGid } : {}),
  };
  buildInstallArtifacts(plan);
  return plan;
}
