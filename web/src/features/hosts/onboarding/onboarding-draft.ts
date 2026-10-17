import type { AgentInstallPlan } from "@/domain/agent-install";
import { AGENT_PORT } from "@/domain/agent-install";
import type { HostDefaults } from "@/domain/settings";
import { absolutePath, hostAddress, hostName, installDirProblem, normalizePath, optionalId, port, type InstallDirProblem, type ValidationKey } from "@/domain/validation";

import type { HostDraft } from "./onboarding-store";

export { onboardingStore, patchDraft, useOnboardingDraft, type HostDraft } from "./onboarding-store";

export const ONBOARDING_STEPS = ["connection", "directories", "install", "connect"] as const;
export type OnboardingStep = (typeof ONBOARDING_STEPS)[number];

export const HOST_COLORS = ["#8b5cf6", "#22c55e", "#eab308", "#ec4899", "#06b6d4", "#ef4444", "#84cc16", "#6366f1"];

export function freshDraft(defaults: HostDefaults, usedColors: readonly string[]): HostDraft {
  return {
    name: "",
    color: HOST_COLORS.find((c) => !usedColors.includes(c)) ?? HOST_COLORS[0] ?? "#8b5cf6",
    connection: defaults.connection,
    address: "",
    port: String(AGENT_PORT),
    projectDir: defaults.projectDir,
    installDir: defaults.installDir,
    manualIds: !!(defaults.puid || defaults.pgid),
    puid: defaults.puid,
    pgid: defaults.pgid,
    detectSocket: defaults.detectSocketGroup,
    socketGid: "",
    simulateFailure: false,
    reached: 0,
  };
}

export type DraftErrors = Partial<Record<"name" | "address" | "port" | "projectDir" | "installDir" | "puid" | "pgid" | "socketGid" | "hubUrl", ValidationKey | InstallDirProblem>>;

export function validateStep(step: OnboardingStep, d: HostDraft, takenNames: readonly string[]): DraftErrors {
  const e: DraftErrors = {};
  if (step === "connection") {
    e.name = hostName(d.name, takenNames);
    if (d.hubUrl !== undefined) {
      try {
        const u = new URL(d.hubUrl);
        if (!["http:", "https:"].includes(u.protocol) || !u.hostname || u.username || u.password || u.search || u.hash || !["", "/"].includes(u.pathname) || /[\r\n\0]/.test(d.hubUrl)) e.hubUrl = "url";
      } catch { e.hubUrl = "url"; }
    }
    if (d.connection === "direct") {
      e.address = hostAddress(d.address);
      e.port = port(d.port);
    }
  }
  if (step === "directories") {
    e.projectDir = absolutePath(d.projectDir);
    e.installDir = absolutePath(d.installDir);
    // A folder of its own inside the project directory is fine; the root itself, a parent or a stack folder are not.
    if (!e.projectDir && !e.installDir) e.installDir = installDirProblem(d.projectDir, d.installDir);
    if (d.manualIds) {
      e.puid = optionalId(d.puid) ?? (d.puid.trim() ? undefined : "required");
      e.pgid = optionalId(d.pgid) ?? (d.pgid.trim() ? undefined : "required");
    }
    if (!d.detectSocket) e.socketGid = optionalId(d.socketGid) ?? (d.socketGid.trim() ? undefined : "required");
  }
  for (const k of Object.keys(e) as (keyof DraftErrors)[]) if (!e[k]) delete e[k];
  return e;
}

/** Root-owned IDs are allowed but flagged; the UI warns instead of blocking. */
export function usesRootIds(d: HostDraft): boolean {
  return d.manualIds && (d.puid.trim() === "0" || d.pgid.trim() === "0");
}

export function draftFileOwner(d: HostDraft) {
  return d.manualIds && d.puid && d.pgid ? { uid: Number(d.puid), gid: Number(d.pgid) } : undefined;
}

export function planFromDraft(d: HostDraft, hubUrl: string, timeZone: string): AgentInstallPlan {
  return {
    hostName: d.name.trim() || "neuer-host",
    connection: d.connection,
    address: d.address.trim(),
    port: Number(d.port) || AGENT_PORT,
    hubUrl,
    projectDir: normalizePath(d.projectDir),
    installDir: normalizePath(d.installDir),
    fileOwner: draftFileOwner(d),
    socketGid: d.detectSocket ? undefined : Number(d.socketGid),
    token: d.token ?? "DEMO-NUR-ANSICHT",
    timeZone,
  };
}
