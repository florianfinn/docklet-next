// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime type compatibility and existing rendering preservation proofs.

import { expect, expectTypeOf, it } from "vitest";

import type { ActionRun, RuntimeActionRun, RuntimeKnowledge } from "@/domain/model";
import type { ActionReadHealth, CodedRuntimeActionRun, DockletAdapter, RuntimeReleaseInput, Unsubscribe } from "./adapter";
import { AdapterOutcomeError, type AdapterOutcome } from "./capability";
import { renderRun, type CodedActionRun } from "./message-code";

const historical: CodedActionRun = {
  id: "run",
  kind: "start",
  title: { code: "activity.running", params: {} },
  target: { code: "activity.message", params: { message: "Example container" } },
  hostId: "0",
  services: [],
  steps: [],
  status: "running",
  progress: 0,
  startedAt: "2026-10-19T12:00:00Z",
  log: [],
  output: [{ at: "2026-10-19T12:00:01Z", stream: "stdout", text: "Execution output" }],
};

it("keeps historical runs valid without inventing runtime knowledge", () => {
  const run: ActionRun = renderRun(historical, "en-US");
  expect(run.status).toBe("running");
  expect(run).not.toHaveProperty("outcomeState");
  expect(run).not.toHaveProperty("cancelRequested");
  expect(JSON.parse(JSON.stringify(run))).not.toHaveProperty("outcomeState");
  expect(JSON.parse(JSON.stringify(run))).not.toHaveProperty("cancelRequested");
});

it.each(["de", "en-US"] as const)("preserves runtime knowledge through existing coded rendering and serialization in %s", (locale) => {
  for (const state of [
    { status: "running", outcomeState: "unknown", cancelRequested: false },
    { status: "running", outcomeState: "unknown", cancelRequested: true },
    { status: "failed", outcomeState: "unknown", cancelRequested: true },
    { status: "failed", outcomeState: "known", cancelRequested: false },
  ] as const) {
    const coded: CodedRuntimeActionRun = { ...historical, ...state };
    const run = renderRun(coded, locale);
    expect(run).toMatchObject(state);
    expect(JSON.parse(JSON.stringify(run))).toMatchObject(state);
    expect(run.title).toBe(locale === "de" ? "Läuft" : "Running");
    expect(run.output).toEqual(historical.output);
    expect(renderRun(coded, locale)).toBe(run);
  }
});

it("represents mismatch without disclosing an ID while busy and cancelling require one", () => {
  const mismatch = { kind: "conflict", code: "run.request-mismatch" } satisfies AdapterOutcome<never>;
  const error = new AdapterOutcomeError(mismatch);
  expect(error.outcome).toEqual(mismatch);
  expect(JSON.parse(JSON.stringify(error.outcome))).toEqual({ kind: "conflict", code: "run.request-mismatch" });

  // @ts-expect-error Busy conflicts require the authorized active identity.
  const busyWithoutID: AdapterOutcome<never> = { kind: "conflict", code: "run.busy" };
  // @ts-expect-error Cancelling conflicts require the authorized active identity.
  const cancellingWithoutID: AdapterOutcome<never> = { kind: "conflict", code: "run.cancelling" };
  // @ts-expect-error Request mismatch must not carry a bound identity.
  const disclosingMismatch: AdapterOutcome<never> = { kind: "conflict", code: "run.request-mismatch", activeRunId: "run" };
  void [busyWithoutID, cancellingWithoutID, disclosingMismatch];
});

it("keeps runtime projections paired and adapter additions optional without changing commands or read health", () => {
  const optional: Pick<DockletAdapter, "getRuntimeRun" | "cancelRuntimeRun" | "releaseRuntimeRun"> = {};
  expect(optional).toEqual({});
  expectTypeOf<ReturnType<DockletAdapter["runStackCommand"]>>().toEqualTypeOf<Promise<string>>();
  expectTypeOf<ReturnType<DockletAdapter["runContainerCommand"]>>().toEqualTypeOf<Promise<string>>();
  expectTypeOf<ReturnType<DockletAdapter["deployStack"]>>().toEqualTypeOf<Promise<{ actionId: string; stackId: string }>>();
  expectTypeOf<DockletAdapter["subscribeActionReadHealth"]>().toEqualTypeOf<((listener: (health: ActionReadHealth) => void) => Unsubscribe) | undefined>();
  expectTypeOf<RuntimeActionRun>().toExtend<ActionRun>();
  expectTypeOf<CodedRuntimeActionRun>().toExtend<CodedActionRun>();

  // @ts-expect-error Runtime projection knowledge requires explicit cancellation intent.
  const missingIntent: RuntimeKnowledge = { outcomeState: "unknown" };
  // @ts-expect-error Cancellation intent alone does not establish outcome knowledge.
  const missingKnowledge: RuntimeKnowledge = { cancelRequested: false };
  // @ts-expect-error Ordinary historical runs are not known runtime projections.
  const historicalRuntime: CodedRuntimeActionRun = historical;
  // @ts-expect-error Administrator release requires explicit risk acknowledgment.
  const missingAcknowledgment: RuntimeReleaseInput = { reason: "Executor stopped and state inspected" };
  // @ts-expect-error Unknown is knowledge, never an action status.
  const unknownStatus: ActionRun["status"] = "unknown";
  void [missingIntent, missingKnowledge, historicalRuntime, missingAcknowledgment, unknownStatus];
});
