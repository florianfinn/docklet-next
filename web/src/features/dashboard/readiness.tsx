// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original Dashboard readiness markers for load measurement (components).

import type { ReactNode } from "react";

import { ReadyContext, useWidgetReady, type ReadyReport } from "./use-readiness";

export function WidgetReady({ id, ready, children }: { id: string; ready: boolean; children: ReactNode }) {
  useWidgetReady(id, ready);
  return children;
}

export function ReadinessProvider({ report, children }: { report: ReadyReport; children: ReactNode }) {
  return <ReadyContext.Provider value={report}>{children}</ReadyContext.Provider>;
}
