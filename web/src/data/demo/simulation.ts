import { createStore, mergeShallow } from "@/shared/lib/store";

import type { FailAt } from "./options";

export type Latency = "instant" | "short" | "long" | "endless";
export type FailArea = "none" | "all" | "dashboard" | "stacks" | "containers" | "security" | "networks" | "volumes" | "hosts" | "settings" | "images";
export type ReadArea = Exclude<FailArea, "none" | "all">;

export interface SimulationSettings {
  latency: Latency;
  failArea: FailArea;
  empty: boolean;
  live: boolean;
  /** Multiplier for simulated action step durations. */
  actionSpeed: number;
  /** Forces the next started action to fail at this step, then resets. */
  nextActionFails: FailAt;
}

export const DEFAULT_SIMULATION: SimulationSettings = {
  latency: "short",
  failArea: "none",
  empty: false,
  live: true,
  actionSpeed: 1,
  nextActionFails: "none",
};

/** Read by the demo adapter on every call; written by the design lab. */
export const simulationStore = createStore<SimulationSettings>(DEFAULT_SIMULATION, {
  key: "docklet-react-opus.simulation",
  version: 1,
  merge: mergeShallow,
});

const LATENCY_MS: Record<Exclude<Latency, "endless">, number> = { instant: 0, short: 650, long: 2400 };

async function simulatedLatency(settings: SimulationSettings, scale: number): Promise<void> {
  if (settings.latency === "endless") return new Promise<void>(() => undefined);
  const base = LATENCY_MS[settings.latency] * scale;
  if (base > 0) await sleep(base * (0.75 + Math.random() * 0.5));
}

/** `scale` shortens latency for frequent follow-up reads such as folder listings. */
export async function simulateRead<T>(area: ReadArea, produce: () => T, scale = 1): Promise<T> {
  const settings = simulationStore.get();
  await simulatedLatency(settings, scale);
  if (settings.failArea === "all" || settings.failArea === area) {
    throw new SimulatedError(area);
  }
  return produce();
}

/** Small write with the read latency; fails for a failing area or the lab's forced failure (consumed). */
export async function simulateWrite(area: ReadArea): Promise<void> {
  const settings = simulationStore.get();
  await simulatedLatency(settings, 1);
  if (takeForcedFailure() !== "none" || settings.failArea === "all" || settings.failArea === area) throw new SimulatedError(area, "write");
}

export class SimulatedError extends Error {
  readonly area: ReadArea;
  constructor(area: ReadArea, kind: "read" | "write" = "read") {
    super(kind === "read" ? `Simulierter Fehler beim Laden (${area})` : `Simulierter Fehler beim Speichern (${area})`);
    this.area = area;
    this.name = "SimulatedError";
  }
}

export function takeForcedFailure(): FailAt {
  const forced = simulationStore.get().nextActionFails;
  if (forced !== "none") simulationStore.set((s) => ({ ...s, nextActionFails: "none" }));
  return forced;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
