import type { AppLocale } from "@/domain/locale";
import type { ContainerInfo, ScheduledJob, SecurityOverview } from "@/domain/model";
import type { DockletAdapter } from "@/data/adapter";

export { createDisconnectedAdapter as createAdapter } from "@/data/disconnected-adapter";
export interface DemoFailure {
  operation: "runStackCommand" | "installStackUpdates" | "deployStack" | "recreateContainer" | "connectHost" | "pullImage";
  at?: "none" | "pull" | "start" | "health";
}
export function queueDemoFailure(_adapter: DockletAdapter, _failure: DemoFailure): void {}
export { BackendNotice as LoginPage } from "@/data/BackendNotice";

export function DesignLabPanel() { return null; }
export function LiveToggle() { return null; }
export function LabMenuItem() { return null; }
export function SimulationBadge(_props: { className?: string }) { return null; }
export function subscribeLabPopups(_close: () => void): () => void { return () => {}; }

/** Production data never passes through the fixture translation table. */
export function localizeDemoText(text: string, locale: AppLocale): string;
export function localizeDemoText(text: string | undefined, locale: AppLocale): string | undefined;
export function localizeDemoText(text: string | undefined, _locale: AppLocale): string | undefined { return text; }

export function localizeContainers(data: ContainerInfo[], _locale: AppLocale): ContainerInfo[] { return data; }
export function localizeSchedule(data: ScheduledJob[], _locale: AppLocale): ScheduledJob[] { return data; }
export function localizeSecurity(data: SecurityOverview, _locale: AppLocale): SecurityOverview { return data; }
