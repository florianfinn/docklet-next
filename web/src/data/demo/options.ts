import type { DockletAdapter } from "../adapter";
import type { ActionRun } from "@/domain/model";

export type FailAt = "none" | "pull" | "start" | "health";
export type DemoFailureOperation = "runStackCommand" | "installStackUpdates" | "deployStack" | "recreateContainer" | "connectHost" | "pullImage";
export interface DemoFailure {
  operation: DemoFailureOperation;
  at?: FailAt;
}

export interface DemoAdapterOptions {
  nextFailure?: DemoFailure;
  missingCapabilities?: readonly string[];
  seedRuns?: Omit<ActionRun, "output">[];
}

const demoOptions = Symbol("docklet.demo.options");
type ConfiguredAdapter = DockletAdapter & { [demoOptions]?: DemoAdapterOptions };

function stateOf(adapter: DockletAdapter): DemoAdapterOptions | undefined {
  return (adapter as ConfiguredAdapter)[demoOptions];
}

export function registerDemoOptions(adapter: DockletAdapter, initial: DemoAdapterOptions): void {
  Object.defineProperty(adapter, demoOptions, { value: { ...initial }, configurable: false });
}

export function queueDemoFailure(adapter: DockletAdapter, failure: DemoFailure): void {
  if (adapter.kind !== "demo") return;
  const state = stateOf(adapter);
  if (state) state.nextFailure = failure;
}

export function takeDemoFailure(adapter: DockletAdapter, operation: DemoFailureOperation): DemoFailure | undefined {
  const state = stateOf(adapter);
  if (state?.nextFailure?.operation !== operation) return undefined;
  const failure = state.nextFailure;
  state.nextFailure = undefined;
  return failure;
}
