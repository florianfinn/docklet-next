// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original demo origin-admission parity tests against backend Assign.

import { expect, it } from "vitest";
import { AdapterOutcomeError } from "../capability";
import type { ActionRunner } from "./action-runner";
import { createDemoContainerOperations } from "./demo-container-operations";
import { DemoDb } from "./demo-db";

const outcome = (p: Promise<unknown>) => p.then(() => "ok", (e: unknown) => e instanceof AdapterOutcomeError && e.outcome.kind === "error" ? e.outcome.code : "other");

// Backend Assign admits writes only while raw detection is unknown; reset of an
// unconfirmed override is admitted for every detected origin.
it.each(["compose", "standalone", "unraid"] as const)("refuses assignment for an unconfirmed override detected as %s but keeps reset", async (origin) => {
  const db = new DemoDb();
  const seed = db.standalone.find((c) => c.origin === "standalone")!;
  const labels: Record<string, string> = origin === "compose" ? { "com.docker.compose.project": "app", "com.docker.compose.service": "api" } : {};
  db.standalone.push({ ...seed, id: "c-example", name: "example", origin, originSource: "unconfirmed", dockerLabels: labels });
  const { api: ops } = createDemoContainerOperations({ db, runner: {} as ActionRunner, forcedFailure: () => false, demoFailure: () => undefined }, () => undefined);
  expect(await outcome(ops.setContainerOrigin!("c-example", "unraid"))).toBe("container.origin-locked");
  expect(await outcome(ops.setContainerOrigin!("c-example", "standalone"))).toBe("container.origin-locked");
  expect(await outcome(ops.setContainerOrigin!("c-example", "unknown"))).toBe("ok");
  expect(db.containers().find((c) => c.id === "c-example")?.originSource).toBe("detected");
});

it("assigns while detection is unknown and keeps the Compose-label veto for standalone", async () => {
  const db = new DemoDb();
  const seed = db.standalone.find((c) => c.origin === "unknown")!;
  db.standalone.push({ ...seed, id: "c-partial", name: "partial", dockerLabels: { "com.docker.compose.project": "app" } });
  const { api: ops } = createDemoContainerOperations({ db, runner: {} as ActionRunner, forcedFailure: () => false, demoFailure: () => undefined }, () => undefined);
  expect(await outcome(ops.setContainerOrigin!(seed.id, "unraid"))).toBe("ok");
  expect(await outcome(ops.setContainerOrigin!("c-partial", "standalone"))).toBe("container.origin-locked");
  expect(await outcome(ops.setContainerOrigin!("c-partial", "unraid"))).toBe("ok");
});
