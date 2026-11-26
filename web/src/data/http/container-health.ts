// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import type { components } from "@/api/generated/schema";
import type { ContainerHealthDetails } from "@/domain/health";
import type { Health } from "@/domain/model";

export type WireDetails = components["schemas"]["ContainerDetails"];

const NS_PER_MS = 1_000_000;
const ms = (ns: number | undefined) => (typeof ns === "number" && Number.isFinite(ns) && ns > 0 ? Math.round(ns / NS_PER_MS) : 0);
const health = (status: string | undefined): Health => (status === "healthy" || status === "unhealthy" || status === "starting" ? status : "none");

/** Keeps only the health projection: environment, command and other inspect fields never leave this function. */
export function mapContainerHealth(containerId: string, d: WireDetails): ContainerHealthDetails {
  const hc = d.config?.healthcheck;
  const startedAt = d.state.startedAt && !d.state.startedAt.startsWith("0001-") ? d.state.startedAt : undefined;
  return {
    containerId,
    running: d.state.running,
    ...(startedAt ? { startedAt } : {}),
    status: health(d.state.health?.status),
    failingStreak: d.state.health?.failingStreak ?? 0,
    probes: (d.state.health?.log ?? []).map((p) => ({ ...(p.start ? { start: p.start } : {}), ...(p.end ? { end: p.end } : {}), exitCode: p.exitCode, output: p.output ?? "" })),
    ...(hc?.test?.length ? { check: { test: [...hc.test], intervalMs: ms(hc.interval), timeoutMs: ms(hc.timeout), retries: hc.retries ?? 0, startPeriodMs: ms(hc.startPeriod), startIntervalMs: ms(hc.startInterval) } } : {}),
  };
}
