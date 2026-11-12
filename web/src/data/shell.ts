// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original transport-neutral shell session port using generated v11 DTOs.

import type { components } from "@/api/generated/schema";
export type ShellOptions = components["schemas"]["ContainerShellQuery"];
export type ShellResize = components["schemas"]["ContainerShellResize"];
export type ShellClose = components["schemas"]["ContainerShellClose"];
export type ShellExit = components["schemas"]["ContainerShellExit"];
export type ShellEvent = { type: "connecting" | "live" } | { type: "output"; data: Uint8Array } | ShellExit;
export interface ShellSession {
  send(data: string): void;
  resize(cols: number, rows: number): void;
  close(): void;
}
export type OpenShell = (containerId: string, options: ShellOptions, listener: (event: ShellEvent) => void) => ShellSession;
export const SHELL_REASONS = ["container.shell-invalid", "container.shell-protocol", "container.shell-unavailable", "container.shell-orphaned", "container.shell-not-running"] as const;
export function shellExit(value: unknown): ShellExit | undefined {
  if (!value || typeof value !== "object") return;
  const v = value as Partial<ShellExit>;
  if (v.type !== "exit" || (v.code !== null && !Number.isSafeInteger(v.code)) || typeof v.reason !== "string" || v.reason.length > 128) return;
  return v as ShellExit;
}
