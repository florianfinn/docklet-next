// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import type { ContainerHealthApi, ContainerHealthDetails } from "@/domain/health";
import type { ContainerInfo } from "@/domain/model";
import { AdapterOutcomeError } from "../capability";

const CHECK = { test: ["CMD-SHELL", "wget -qO /dev/null http://127.0.0.1/health"], intervalMs: 30_000, timeoutMs: 5_000, retries: 3, startPeriodMs: 0, startIntervalMs: 0 };

/** Demo health: unhealthy containers fail with a printed HTTP 404, healthy ones pass. */
export function createDemoContainerHealth(containers: () => ContainerInfo[]): ContainerHealthApi {
  return {
    async getContainerHealth(containerId): Promise<ContainerHealthDetails> {
      const c = containers().find((x) => x.id === containerId);
      if (!c) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: 404 });
      const running = c.state === "running";
      if (c.health === "none") return { containerId, running, status: "none", failingStreak: 0, probes: [] };
      const at = new Date(Date.now() - 30_000).toISOString();
      const failed = c.health === "unhealthy";
      return { containerId, running, status: c.health, failingStreak: failed ? 12 : 0, check: CHECK,
        probes: c.health === "starting" ? [] : [{ start: at, end: at, exitCode: failed ? 1 : 0, output: failed ? "wget: server returned error: HTTP/1.1 404 Not Found\n" : "" }] };
    },
  };
}
