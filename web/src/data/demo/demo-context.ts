import { AdapterError } from "../api";
import type { ActionRunner } from "./action-runner";
import type { DemoDb } from "./demo-db";
import type { DemoFailure, DemoFailureOperation } from "./options";

/** Demo modules throw the contract's error type so UIs stay adapter-neutral. */
export { AdapterError as DemoError };

export interface DemoContext {
  db: DemoDb;
  /** Fresh runtime admission follows retained idempotency receipts. */
  guardRuntimeHost?: (hostId: string) => void;
  runner: ActionRunner;
  /** Forced failure from the design lab, if any, consumed by the next action. */
  forcedFailure: () => boolean;
  demoFailure: (operation: DemoFailureOperation) => DemoFailure | undefined;
}

export const wait = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

export const failMessage = "Simulierter Fehler (Design-Labor)";
