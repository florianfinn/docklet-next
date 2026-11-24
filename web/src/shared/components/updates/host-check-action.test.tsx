// Source: docklet-next/issues/597
// Revision: 6e3c477e97192029f67bcf924da27b3b3849bbd8
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original host check busy, refreshed inventory and navigation proofs.

import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapActivityDetail } from "@/data/http/map-dto";
import type { CodedActionRun } from "@/data/message-code";
import { deferred } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";
import { setAppLocale } from "@/app/i18n/locale-store";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { queryKeys } from "@/data/query-keys";
import { ActivityCenter } from "@/features/activity/ActivityCenter";
import { activityStore } from "@/features/activity/activity-store";
import { HostCheckAction } from "./host-check-action";
import { AdapterOutcomeError } from "@/data/capability";
import type { ContainerInfo, Stack } from "@/domain/model";
import activity from "@/features/stacks/testdata/host-update-activity.json";

const updated = { state: "rebuild" as const };
const stack = (id: string, hostId = "example-host", pending = true) => ({ id, hostId, services: [{ update: pending ? updated : { state: "current" } }] }) as Stack;
const container = (id: string, hostId = "example-host", stackId?: string) => ({ id, hostId, stackId, update: updated }) as ContainerInfo;
afterEach(() => { cleanup(); vi.restoreAllMocks(); activityStore.reset(); setAppLocale("de"); });

function fixture() {
  const adapter = createDemoAdapter(), listeners = new Set<(rows: CodedActionRun[]) => void>();
  let rows: CodedActionRun[] = [], client!: QueryClient;
  const publish = (status: CodedActionRun["status"], partial = false) => {
    rows = [{ ...mapActivityDetail({ activity: partial ? { ...activity, status: "failed", metadata: { ...activity.metadata, jobStatus: "partial" } } : activity, stepTrace: [], messages: [], output: [], outputTruncated: false } as never), status }];
    listeners.forEach((f) => f(rows));
  };
  adapter.subscribeActions = (f) => { listeners.add(f); f(rows); return () => { listeners.delete(f); }; };
  adapter.listActions = async () => rows;
  adapter.checkHostUpdates = vi.fn(async () => { publish("queued"); return activity.id; });
  adapter.listStacks = vi.fn(async () => [stack("a"), stack("b"), stack("none", "example-host", false), stack("foreign", "other")]);
  adapter.listContainers = vi.fn(async () => [container("one"), container("compose", "example-host", "a"), container("foreign", "other")]);
  function Surface({ hostId = "example-host" }: { hostId?: string }) { client = useQueryClient(); return <HostCheckAction hostId={hostId} />; }
  return { adapter, publish, Surface, client: () => client, listeners };
}

it.each(["en-US", "de"] as const)("stays busy through queued/running and the inventory read, then refreshes badges and counts (%s)", async (locale) => {
  setAppLocale(locale); const f = fixture(), success = vi.spyOn(toast, "success").mockImplementation(() => "example");
  const inventory = deferred<Stack[]>(); f.adapter.listStacks = vi.fn(() => inventory.promise);
  renderWithApp(<f.Surface />, f.adapter);
  const button = await screen.findByRole("button", { name: locale === "de" ? "Auf Updates prüfen" : "Check for updates" });
  f.client().setQueryData(queryKeys.stacks, [stack("cached")]);
  f.client().setQueryData(queryKeys.containers, []);
  await userEvent.click(button);
  expect(button.getAttribute("aria-busy")).toBe("true"); expect(button.hasAttribute("disabled")).toBe(true);
  expect(f.adapter.listStacks).not.toHaveBeenCalled();
  act(() => f.publish("running"));
  await userEvent.click(button); expect(f.adapter.checkHostUpdates).toHaveBeenCalledTimes(1);
  act(() => f.publish("succeeded"));
  await waitFor(() => expect(f.adapter.listStacks).toHaveBeenCalledTimes(1));
  expect(button.getAttribute("aria-busy")).toBe("true"); expect(success).not.toHaveBeenCalled();
  await act(async () => { inventory.resolve([stack("fresh-a"), stack("fresh-b"), stack("other", "other")]); await inventory.promise; });
  await waitFor(() => expect(success).toHaveBeenCalledTimes(1));
  expect(success).toHaveBeenCalledWith(locale === "de" ? "2 Stacks und 1 Container haben Updates" : "2 stacks and 1 container have updates");
  expect(f.client().getQueryData<Stack[]>(queryKeys.stacks)?.[0]?.id).toBe("fresh-a");
  expect(button.getAttribute("aria-busy")).toBe("false");
});

it("an active host job from another tab disables the host control", async () => {
  const f = fixture(); f.publish("running"); renderWithApp(<f.Surface />, f.adapter);
  const button = await screen.findByRole("button", { name: "Prüfung läuft…" });
  expect(button.hasAttribute("disabled")).toBe(true);
  await userEvent.click(button); expect(f.adapter.checkHostUpdates).not.toHaveBeenCalled();
});

it("a late inventory result after navigation writes no cache or toast", async () => {
  const f = fixture(), inventory = deferred<Stack[]>(), success = vi.spyOn(toast, "success").mockImplementation(() => "example");
  f.adapter.listStacks = vi.fn(() => inventory.promise);
  const view = renderWithApp(<f.Surface />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Auf Updates prüfen" }));
  act(() => f.publish("succeeded"));
  await waitFor(() => expect(f.adapter.listStacks).toHaveBeenCalledTimes(1));
  await act(async () => { await view.router.navigate("/elsewhere"); });
  await act(async () => { inventory.resolve([stack("late")]); await inventory.promise; });
  expect(f.client().getQueryData(queryKeys.stacks)).toBeUndefined(); expect(success).not.toHaveBeenCalled();
});

it.each(["en-US", "de"] as const)("shows the real producer in the Action Centre with its host (%s)", async (locale) => {
  setAppLocale(locale); const f = fixture(); f.publish("queued");
  f.adapter.listHosts = async () => [{ id: "example-host", name: "Example Host" }] as never;
  activityStore.set((s) => ({ ...s, open: true, tab: "actions" }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await waitFor(() => expect(document.querySelector("#run-example-host-check")?.textContent).toContain(" · Example Host"));
  expect(document.querySelector("#run-example-host-check")?.textContent).toContain(locale === "de" ? "Update-Prüfung" : "Update check");
});

it("failed checks still refresh successful badges and report failure instead of a success count", async () => {
 const f = fixture(), success = vi.spyOn(toast, "success").mockImplementation(() => "example"), error = vi.spyOn(toast, "error").mockImplementation(() => "example");
 renderWithApp(<f.Surface />, f.adapter);
 await userEvent.click(await screen.findByRole("button", { name: "Auf Updates prüfen" }));
 act(() => f.publish("failed"));
 await waitFor(() => expect(error).toHaveBeenCalledTimes(1));
 expect(f.client().getQueryData<Stack[]>(queryKeys.stacks)?.length).toBe(4); expect(success).not.toHaveBeenCalled();
});

// D-7: a partial run reports its counts with the number of targets that could not be checked and a link to the run.
it("partial checks report the counts and how many targets could not be checked", async () => {
 const f = fixture(), warning = vi.spyOn(toast, "warning").mockImplementation(() => "example"), error = vi.spyOn(toast, "error").mockImplementation(() => "example");
 f.adapter.getUpdateJobTargets = vi.fn(async () => [{ id: "a", status: "succeeded" as const }, { id: "b", status: "failed" as const }, { id: "c", status: "failed" as const }]);
 renderWithApp(<f.Surface />, f.adapter);
 await userEvent.click(await screen.findByRole("button", { name: "Auf Updates prüfen" }));
 act(() => f.publish("failed", true));
 await waitFor(() => expect(warning).toHaveBeenCalledTimes(1));
 expect(String(warning.mock.calls[0]?.[0])).toBe("2 Stacks und 1 Container haben Updates");
 expect((warning.mock.calls[0]?.[1] as { description?: string }).description).toBe("2 Ziele konnten nicht geprüft werden.");
 expect(error).not.toHaveBeenCalled();
});

it("navigation while queued cancels the completion listener before inventory writes", async () => {
 const f = fixture(), success = vi.spyOn(toast, "success").mockImplementation(() => "example");
 const view = renderWithApp(<f.Surface />, f.adapter);
 await userEvent.click(await screen.findByRole("button", { name: "Auf Updates prüfen" }));
 await act(async () => { await view.router.navigate("/elsewhere"); });
 await act(async () => { f.publish("succeeded"); await Promise.resolve(); });
 expect(f.adapter.listStacks).not.toHaveBeenCalled(); expect(success).not.toHaveBeenCalled();
});

it("identity replacement discards an old inventory result", async () => {
 const f = fixture(), inventory = deferred<Stack[]>(), success = vi.spyOn(toast, "success").mockImplementation(() => "example");
 let generation = 0; const listeners = new Set<() => void>();
 registerIdentityLifetime(f.adapter.subscribeChanges, { version: () => generation, active: () => true, listen: (f) => { listeners.add(f); return () => { listeners.delete(f); }; } });
 f.adapter.listStacks = vi.fn(() => inventory.promise);
 renderWithApp(<f.Surface />, f.adapter);
 await userEvent.click(await screen.findByRole("button", { name: "Auf Updates prüfen" }));
 act(() => f.publish("succeeded")); await waitFor(() => expect(f.adapter.listStacks).toHaveBeenCalledTimes(1));
 await act(async () => { generation++; listeners.forEach((f) => f()); await Promise.resolve(); });
 await act(async () => { inventory.resolve([stack("old-identity")]); await inventory.promise; });
 expect(f.client().getQueryData(queryKeys.stacks)).toBeUndefined(); expect(success).not.toHaveBeenCalled();
});

it("shows a failed admission as unfinished and stays usable", async () => {
  const f = fixture();
  f.adapter.checkHostUpdates = vi.fn(async () => { throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }); });
  renderWithApp(<f.Surface />, f.adapter);
  const button = await screen.findByRole("button", { name: "Auf Updates prüfen" });
  await userEvent.click(button);
  await waitFor(() => expect(screen.queryByRole("alert") !== null).toBe(true));
  expect(screen.getByRole("alert").textContent).toBe("Die Prüfung wurde nicht abgeschlossen. Bitte erneut versuchen.");
  expect(button.textContent).toBe("Auf Updates prüfen");
  expect(button.hasAttribute("disabled")).toBe(false);
});

it("is busy while an all-hosts check runs", async () => {
  const f = fixture();
  renderWithApp(<f.Surface />, f.adapter);
  const button = await screen.findByRole("button", { name: "Auf Updates prüfen" });
  expect(button.hasAttribute("disabled")).toBe(false);
  act(() => f.listeners.forEach((listen) => listen([{ ...mapActivityDetail({ activity: { ...activity, id: "example-all", resourceId: "update-check", metadata: {} }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never), status: "running", hostId: "0", jobRef: undefined, stackId: undefined, containerId: undefined, kind: "check-updates" }])));
  expect(button.getAttribute("aria-busy")).toBe("true");
  expect(button.hasAttribute("disabled")).toBe(true);
});
