// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import { fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider, createTranslator } from "use-intl";
import { expect, vi } from "vitest";
import { toast } from "sonner";
import type { UseQueryResult } from "@tanstack/react-query";
import { catalogs } from "@/app/i18n/catalog";
import { renderWithApp } from "@/test/render";
import { useImageError } from "@/features/images/use-image-error";
import { useNetworkCommands } from "@/features/networks/use-network-commands";
import { useVolumeCommands } from "@/features/volumes/use-volume-commands";
import { BackupOperation } from "@/features/volumes/backups/BackupOperation";
import { BackupTargetsSection } from "@/features/settings/sections/BackupTargetsSection";
import { NotificationsSection } from "@/features/settings/sections/NotificationsSection";
import { HostUpdateSettings } from "@/features/settings/sections/LiveUpdatesSection";
import HubBackupsPage from "@/features/hub-backups/HubBackupsPage";
import { PruneVolumesDialog } from "@/features/volumes/PruneVolumesDialog";
import { AdvisoryContent } from "@/features/security/AdvisoryContent";
import { OverviewTab } from "@/features/security/SecurityTabs";
import { HistorySection } from "@/shared/history/HistorySection";
import type { RuntimeHistory } from "@/domain/history";
import type { Volume } from "@/domain/volumes";
import { createDemoAdapter } from "./demo/demo-adapter";
import { AdapterError, type AdapterErrorCode } from "./api";
import { AdapterOutcomeError } from "./capability";
import { useCommandError } from "./use-command-error";
import { renderRun, type CodedActionRun } from "./message-code";
import { createHostRestarts } from "./http/host-restarts";

export const de = createTranslator({ locale: "de", messages: catalogs.de });
export const en = createTranslator({ locale: "en-US", messages: catalogs["en-US"] });
export const at = "2026-11-06T12:00:00Z";
export const available = async (operationId: string, resourceRef: { kind: string; id: string }) => ({ operationId, resourceRef, available: true });
const intl = ({ children }: { children: React.ReactNode }) => <IntlProvider locale="de" messages={catalogs.de}>{children}</IntlProvider>;

export function imageError(code: string) {
  return renderHook(() => useImageError(), { wrapper: intl }).result.current(code);
}
export function commandError(code: string) {
  return renderHook(() => useCommandError(), { wrapper: intl }).result.current(new AdapterOutcomeError({ kind: "error", code }));
}

export async function volumeOrNetworkError(feature: "volume" | "network", code: string) {
  const adapter = createDemoAdapter();
  const reject = async () => { throw code === "volume.exists" ? new AdapterError("exists") : feature === "network" ? new AdapterError(code as AdapterErrorCode) : new AdapterOutcomeError({ kind: "error", code }); };
  const error = vi.spyOn(toast, "error").mockImplementation(() => "example-toast");
  function Probe() {
    const volume = useVolumeCommands(), network = useNetworkCommands();
    return <button onClick={() => void (feature === "volume" ? volume.create({ hostId: "example", name: "example", labels: {} }) : network.remove("example~network"))}>{de("volumes.create")}</button>;
  }
  renderWithApp(<Probe />, { ...adapter, createVolume: reject, removeNetwork: reject });
  fireEvent.click(screen.getByRole("button", { name: de("volumes.create") }));
  await waitFor(() => expect(error).toHaveBeenCalledOnce());
  return String(error.mock.calls[0]?.[0]);
}

export async function backupStatus(status: string) {
  const adapter = createDemoAdapter();
  adapter.observeActionDetail = (_host, _id, notify) => { notify({ status: "ready", run: { status: "succeeded", progress: 100, backupOutcome: { status } } as never }); return () => {}; };
  const view = renderWithApp(<BackupOperation hostId="example" activityId="example-run" />, adapter);
  await screen.findByRole("status");
  return view.container.textContent ?? "";
}

export async function backupTargets() {
  const adapter = createDemoAdapter();
  const host = (await adapter.listHosts())[0]!;
  adapter.getCapability = available as typeof adapter.getCapability;
  adapter.listBackupTargets = async () => [{ id: "example-target", name: "Example target", kind: "local", path: "/srv/example", hostId: host.id, delivery: [{ hostId: host.id, state: "conflict" }] } as never];
  const view = renderWithApp(<BackupTargetsSection />, adapter);
  await screen.findByText(catalogs.de.settings.backups.deliveryConflict, { exact: false });
  return view.container.textContent ?? "";
}

export async function hubBackup(maintenance: boolean) {
  const adapter = createDemoAdapter({ hubBackupScenario: maintenance ? "maintenance" : "list" });
  const view = renderWithApp(<HubBackupsPage />, adapter);
  await screen.findByText(maintenance ? catalogs.de.hubBackups.maintenancePhase.draining : catalogs.de.hubBackups.statusValues.succeeded);
  return view.container.textContent ?? "";
}

async function restartRun(row: Record<string, string>) {
  let run: CodedActionRun | undefined;
  const api = createHostRestarts({ baseUrl: "/api", client: { fetch: async () => Response.json({ success: true, data: { runId: "example-run", createdAt: at, deadline: at, ...row } }), token: async () => "example" }, capability: available as never, generation: () => 0, publish: (value) => { run = value; }, changed: () => {} });
  try {
    await api.restartAgent("example");
    return renderRun(run!, "de");
  } finally { api.reset(); }
}
export async function restartPhase() {
  return (await restartRun({ state: "reserved" })).steps[0]!.label;
}
export async function restartFailure(code: string) {
  // The hub sets settledAt with every terminal state (restart_controller.go persist/markSent).
  return (await restartRun({ state: "failed", code, settledAt: at })).error;
}

export function historyState() {
  const data: RuntimeHistory = { range: "24h", buckets: [{ start: at, end: at, state: "unhealthy", minutes: { running: 0, unhealthy: 15, down: 0, stopped: 0 } }], uptimePercent: 0, totals: { netRx: 0, netTx: 0, diskRead: 0, diskWrite: 0 } };
  renderWithApp(<HistorySection range="24h" onRange={() => {}} query={{ data, isError: false } as UseQueryResult<RuntimeHistory>} />);
  return screen.getByRole("listitem", { name: /Ungesund/ }).getAttribute("aria-label") ?? "";
}

export async function prune(outcome: boolean) {
  const adapter = createDemoAdapter();
  const host = (await adapter.listHosts())[0]!;
  const candidate: Volume = { id: `${host.id}~example`, name: "example", hostId: host.id, kind: "volume", source: "", sizeBytes: 0, createdAt: at, consumers: [], labels: {} };
  adapter.getCapability = available as typeof adapter.getCapability;
  adapter.planVolumePrune = async () => ({ candidates: [candidate], candidateReasons: { [candidate.id]: "unused-anonymous" }, kept: [], skippedHosts: [], all: false });
  adapter.pruneVolumes = async () => ({ removed: [], skipped: [{ id: candidate.id, reason: "changed" }], failed: [{ id: candidate.id, name: "example", hostId: host.id, code: "volume.remove-failed" }], spaceReclaimed: 0 });
  renderWithApp(<PruneVolumesDialog open onOpenChange={() => {}} hosts={[host]} />, adapter);
  await screen.findByText(catalogs.de.volumes.prune.candidateReasons["unused-anonymous"], { exact: false });
  if (outcome) {
    const button = screen.getByRole("button", { name: de("volumes.prune.confirm", { count: 1 }) });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
    await userEvent.click(button);
    await screen.findByText(catalogs.de.volumes.prune.failedReasons["remove-failed"], { exact: false });
  }
  return screen.getByRole("dialog").textContent ?? "";
}

export function scanStatus() {
  const view = renderWithApp(<OverviewTab images={[]} findings={[]} history={[{ id: "example-run", hostId: "example", at, trigger: "manual", status: "interrupted", counts: { critical: 0, high: 0, medium: 0, low: 0, unknown: 0 } }]} stacks={[]} hosts={new Map()} animate={false} onFocus={() => {}} />);
  return view.container.textContent ?? "";
}

export async function systemHint() {
  const adapter = createDemoAdapter();
  adapter.getCapability = available as typeof adapter.getCapability;
  adapter.getSystemImageHints = async () => ({ hubVersion: "1.0.0", images: [{ kind: "tools", reference: "example/component:1.0.0", state: "available", manualOnly: true, requiresHubUpdate: false }] });
  adapter.listHostRestorePoints = async () => [];
  const view = renderWithApp(<HostUpdateSettings hostId="example" name="Example" />, adapter);
  await screen.findByText(catalogs.de.common.stackUpdates.hint.available, { exact: false });
  return view.container.textContent ?? "";
}

export async function notificationResult() {
  const adapter = createDemoAdapter();
  adapter.getCapability = available as typeof adapter.getCapability;
  adapter.testNotificationTarget = async () => ({ code: "notification.test-failed", reason: "delivery-auth" });
  const error = vi.spyOn(toast, "error").mockImplementation(() => "example-toast");
  renderWithApp(<NotificationsSection />, adapter);
  const button = (await screen.findAllByRole("button", { name: catalogs.de.settings.notifications.test }))[0]!;
  await userEvent.click(button);
  await waitFor(() => expect(error).toHaveBeenCalledOnce());
  return String(error.mock.calls[0]?.[0]);
}

export function advisoryUsage() {
 return renderWithApp(<AdvisoryContent data={{key:"example",hosts:[{hostId:"example",detail:{occurrences:[],usage:{state:"unavailable",code:"image.usage-unavailable"}}}]}} hosts={new Map()} onRetry={()=>{}} />).container.textContent ?? "";
}
