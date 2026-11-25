// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import { describe, expect, it } from "vitest";
import { classifyProbe, diagnoseHealth, firstProbeDue, OVERDUE_SLACK_MS, probeCommand, type ContainerHealthDetails, type HealthCheckConfig, type HealthProbe } from "./health";

// Outputs captured from Docker 28 health logs in the sandbox (alpine 3.22 / BusyBox); the wget 404 line is the
// same BusyBox producer observed on a live probe. Only the curl (22) form is source-verified (curl tool_operate.c).
const OUT = {
  wget404: "wget: server returned error: HTTP/1.1 404 Not Found\n",
  curl404: "curl: (22) The requested URL returned error: 404\n",
  refused: "wget: can't connect to remote host (127.0.0.1): Connection refused\n",
  shellMissing: "/bin/sh: curl: not found\n",
  execMissing: 'OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH',
  timeout: "Health check exceeded timeout (1s)",
  other: "db not ready\n",
};

const STARTED = "2026-11-25T22:02:55.000000000Z";
const check = (test: string[], extra: Partial<HealthCheckConfig> = {}): HealthCheckConfig => ({ test, intervalMs: 2_000, timeoutMs: 0, retries: 0, startPeriodMs: 0, startIntervalMs: 0, ...extra });
const probe = (exitCode: number, output: string, start = "2026-11-25T22:03:03.801124551Z"): HealthProbe => ({ start, end: start, exitCode, output });
const details = (extra: Partial<ContainerHealthDetails>): ContainerHealthDetails => ({
  containerId: "h~abc", running: true, startedAt: STARTED, status: "unhealthy", failingStreak: 8, probes: [], check: check(["CMD-SHELL", "true"]), ...extra,
});
const NOW = Date.parse("2026-11-25T22:04:00Z");

describe("classifyProbe", () => {
  it.each([
    ["wget HTTP status", probe(1, OUT.wget404), { reason: "http-status", httpStatus: 404 }],
    ["curl HTTP status", probe(22, OUT.curl404), { reason: "http-status", httpStatus: 404 }],
    ["connection refused", probe(1, OUT.refused), { reason: "connection-refused" }],
    ["CMD-SHELL tool missing", probe(127, OUT.shellMissing), { reason: "tool-missing", tool: "curl" }],
    ["CMD tool missing", probe(-1, OUT.execMissing), { reason: "tool-missing", tool: "curl" }],
    ["timeout", probe(-1, OUT.timeout), { reason: "timeout" }],
    ["unknown output", probe(3, OUT.other), { reason: "failing" }],
    ["silent failure", probe(1, ""), { reason: "failing" }],
  ])("%s", (_name, p, expected) => {
    const { probe: _p, ...result } = classifyProbe(p);
    expect(result).toEqual(expected);
  });

  it("does not read a missing tool from a shell 'not found' line without exit 127", () => {
    expect(classifyProbe(probe(1, "/bin/sh: page: not found\n")).reason).toBe("failing");
  });
});

describe("diagnoseHealth", () => {
  it("a stopped container keeps Docker's last unhealthy status with an empty log: not running wins", () => {
    expect(diagnoseHealth(details({ running: false, probes: [] }), NOW).reason).toBe("not-running");
  });

  it("separates no healthcheck from a disabled one", () => {
    expect([diagnoseHealth(details({ status: "none", check: undefined }), NOW).reason, diagnoseHealth(details({ status: "none", check: check(["NONE"]) }), NOW).reason]).toEqual(["no-healthcheck", "disabled"]);
  });

  it("reads the newest failed probe of an unhealthy container", () => {
    const d = details({ probes: [probe(1, OUT.refused, "2026-11-25T22:03:01Z"), probe(1, OUT.wget404, "2026-11-25T22:03:03Z")] });
    expect(diagnoseHealth(d, NOW)).toMatchObject({ reason: "http-status", httpStatus: 404, probe: { output: OUT.wget404 } });
  });

  it("ignores probes from before the current start", () => {
    const d = details({ status: "starting", probes: [probe(1, OUT.wget404, "2026-11-25T22:00:00Z")] });
    expect(diagnoseHealth(d, Date.parse(STARTED) + 1_000).reason).toBe("pending");
  });

  it("an unhealthy container whose logged probes all predate the current start gets no earlier-run reason", () => {
    const d = details({ probes: [probe(1, OUT.wget404, "2026-11-25T22:00:00Z")] });
    expect(diagnoseHealth(d, NOW)).toEqual({ reason: "failing" });
  });

  it("drops probes without a readable start and keeps nanosecond order at the start boundary", () => {
    const started = "2026-11-25T22:02:55.000000500Z";
    const missing: HealthProbe = { exitCode: 1, output: OUT.wget404 };
    const before = probe(1, OUT.refused, "2026-11-25T22:02:55.000000400Z");
    const at = probe(-1, OUT.timeout, "2026-11-25T22:02:55.000000500Z");
    expect([
      diagnoseHealth(details({ startedAt: started, probes: [missing] }), NOW).reason,
      diagnoseHealth(details({ startedAt: started, probes: [before] }), NOW).reason,
      diagnoseHealth(details({ startedAt: started, probes: [before, at] }), NOW).reason,
    ]).toEqual(["failing", "failing", "timeout"]);
  });

  it("explains failing probes while still starting", () => {
    expect(diagnoseHealth(details({ status: "starting", probes: [probe(-1, OUT.timeout)] }), NOW).reason).toBe("timeout");
  });

  it("is healthy without a hint", () => {
    expect(diagnoseHealth(details({ status: "healthy", probes: [probe(0, "")] }), NOW)).toEqual({ reason: "healthy" });
  });

  it("a long interval stays pending until its first probe is due, then turns overdue after the slack", () => {
    const d = details({ status: "starting", failingStreak: 0, check: check(["CMD", "true"], { intervalMs: 3_600_000 }) });
    const due = firstProbeDue(d.check!, STARTED);
    expect(due).toBe(Date.parse(STARTED) + 3_600_000 + 30_000);
    expect([diagnoseHealth(d, due + OVERDUE_SLACK_MS).reason, diagnoseHealth(d, due + OVERDUE_SLACK_MS + 1).reason]).toEqual(["pending", "pending-overdue"]);
  });

  it("uses the start interval during a start period", () => {
    const c = check(["CMD", "true"], { intervalMs: 3_600_000, startPeriodMs: 60_000, startIntervalMs: 2_000, timeoutMs: 1_000 });
    expect(firstProbeDue(c, STARTED)).toBe(Date.parse(STARTED) + 3_000);
  });

  it("an unhealthy container without any probe falls back to failing", () => {
    expect(diagnoseHealth(details({ probes: [] }), NOW)).toEqual({ reason: "failing" });
  });
});

describe("probeCommand", () => {
  it("shows the shell text for CMD-SHELL and quotes CMD arguments only where needed", () => {
    expect([probeCommand(check(["CMD-SHELL", 'echo "db not ready"; exit 3'])), probeCommand(check(["CMD", "curl", "-fsS", "http://127.0.0.1/", "a b"]))])
      .toEqual(['echo "db not ready"; exit 3', 'curl -fsS http://127.0.0.1/ "a b"']);
  });
});
