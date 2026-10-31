// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host read projection; persisted colour and palette defaults. M14 host maintenance and availability.

import type { components } from "@/api/generated/schema";
import type { AgentReport, Host, HostStatus } from "@/domain/model";

import { defaultHostColor } from "@/domain/hosts";

type Environment = components["schemas"]["EnvironmentEnvironment"];

function mapStatus(status: string): HostStatus {
  switch (status) {
    case "standby": case "online": case "pending": case "update-required": case "token-rejected": return status;
    case "offline": case "error": return "offline";
    default: throw new Error("hosts.invalid-status");
  }
}

function mapReport(row: Environment): AgentReport | undefined {
  const report = row.agentReport;
  if (!report) return undefined;
  if (!report.version || typeof report.reportedAt !== "string" || !Number.isFinite(Date.parse(report.reportedAt))) throw new Error("hosts.invalid-report");
  const checks = report.version.onboardingChecks;
  if (checks) {
    for (const value of [checks.ownerUid, checks.ownerGid, checks.socketGroup]) {
      if (value != null && (!Number.isInteger(value) || value < 0)) throw new Error("hosts.invalid-checks");
    }
    if (checks.writable != null && typeof checks.writable !== "boolean") throw new Error("hosts.invalid-checks");
  }
  if (!report || !checks || checks.ownerUid == null || checks.ownerGid == null || checks.writable == null) return undefined;
  return {
    reportedAt: report.reportedAt,
    projectDirOwner: { uid: checks.ownerUid, gid: checks.ownerGid, name: `${checks.ownerUid}:${checks.ownerGid}` },
    projectDirWritable: checks.writable,
    ...(checks.socketGroup == null ? {} : { socketGid: checks.socketGroup, socketGidSource: "detected" as const }),
    note: checks.ownerUid === 0 ? "root-owned" : "ok",
    simulated: false,
  };
}

export function mapHost(row: Environment): Host {
  if (!row || typeof row.id !== "string" || !row.id || typeof row.apiUrl !== "string" || typeof row.enabled !== "boolean") throw new Error("hosts.invalid-response");
  if (row.lastSeen && !Number.isFinite(Date.parse(row.lastSeen))) throw new Error("hosts.invalid-last-seen");
  const status = mapStatus(row.status);
  const report = mapReport(row);
  const checks = row.agentReport?.version.onboardingChecks;
  return {
    id: row.id, name: row.name || row.id, color: row.color ?? defaultHostColor(row.id),
    status: row.enabled ? status : "offline",
    connection: row.isEdge ? "edge" : "direct", address: row.isEdge ? "" : row.apiUrl,
    agentVersion: row.agentReport?.version.currentVersion ?? "",
    ...(row.agentReport ? { componentVersion: row.agentReport.version } : {}),
    projectDir: checks?.projectDirectory ?? "", installDir: "",
    os: "", dockerVersion: "", cpuCores: 0, memTotalGb: 0, diskTotalGb: 0,
    detailsUnavailable: true,
    lastSeen: row.lastSeen ?? "",
    ...(report ? { agentReport: report } : {}),
  };
}
