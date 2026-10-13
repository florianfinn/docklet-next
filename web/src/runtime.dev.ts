export { createDemoAdapter as createAdapter } from "@/data/demo/demo-adapter";
export { localizeDemoText, localizeRun, localizeContainers, localizeEvents, localizeSchedule, localizeSecurity } from "@/data/demo/demo-text";
export { DesignLabPanel } from "@/features/design-lab/DesignLabPanel";
export { LiveToggle } from "@/features/design-lab/LiveToggle";
export { LabMenuItem } from "@/features/design-lab/LabMenuItem";
export { SimulationBadge } from "@/features/design-lab/SimulationBadge";
export { default as LoginPage } from "@/features/design-lab/LoginDemoPage";

import { labOpenStore } from "@/features/design-lab/lab-open-store";

export function subscribeLabPopups(close: () => void): () => void {
  return labOpenStore.subscribe(() => labOpenStore.get() && close());
}
