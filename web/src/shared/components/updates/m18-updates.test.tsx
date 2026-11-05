// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-source update gestures, policy locks, recovery and terminal-stream guards.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useQuery } from "@tanstack/react-query";
import { it, expect, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture, point, storedPolicy } from "@/data/http/stack-updates.test-support";
import { mapContainer, type ContainerRow } from "@/data/http/inventory-mapping";
import { mapActivitySummary } from "@/data/http/map-dto";
import { response } from "@/data/http/runtime-commands.test-support";
import { renderRun } from "@/data/message-code";
import type { ContainerInfo } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { ContainerUpdates, ContainerUpdateDialog } from "@/features/containers/ContainerUpdates";
import { ConnectedUpdatesSection } from "@/features/settings/sections/ConnectedUpdatesSection";
import { hasDeployRecovery } from "@/domain/deploy-recovery";
import { DeployRecovery } from "./deploy-recovery";
import { PolicyControl } from "./policy-control";
import { StackUpdateDialog } from "./stack-update-dialog";

const t = catalogs.de.common.stackUpdates;
const container: ContainerInfo = { id: "direct~example-id", hostId: "direct", name: "example", origin: "standalone", image: { repository: "example", tag: "1.0.0" }, state: "running", health: "none", ports: [], networks: [], mounts: [], env: {}, restartPolicy: "no", memLimitMb: 0, update: { state: "rebuild", current: { repository: "example", tag: "1.0.0" }, latest: { repository: "example", tag: "1.0.0" }, registry: "docker.io", checkedAt: point.createdAt } };

it.each(["en-US", "de"] as const)("goal 1: approved connected settings controls save and check globally in %s", async (locale) => {
  setAppLocale(locale); const f = updateFixture(); await f.adapter.getCurrentUser();
  renderWithApp(<ConnectedUpdatesSection />, f.adapter);
  const msg = catalogs[locale].settings.updates;
  const check = await screen.findByRole("button", { name: msg.checkNow });
  await waitFor(() => expect(check.hasAttribute("disabled")).toBe(false));
  await userEvent.click(check);
  await waitFor(() => expect(f.writes().filter((w) => w.path === "/api/updater/check-all")).toHaveLength(1));
  const interval = screen.getByRole("combobox", { name: msg.interval }); await waitFor(() => expect(interval.hasAttribute("disabled")).toBe(false)); interval.focus(); await userEvent.keyboard("{Enter}"); await userEvent.click(await screen.findByRole("option", { name: msg.intervals["6h"] }));
  await userEvent.click(screen.getByRole("switch", { name: msg.notify }));
  await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Speichern" : "Save" }));
  await waitFor(() => expect(f.writes().filter((w) => w.init.method === "PUT" && w.path.endsWith("/settings"))).toHaveLength(1));
  expect(JSON.parse(String(f.writes().find((w) => w.path.endsWith("/settings"))!.init.body))).toEqual({ updateCheckInterval: "6h", updateNotify: "true" });
  expect(document.body.textContent).toContain("UTC");
  expect(screen.queryByRole("textbox", { name: /cron/i }) === null).toBe(true);
});

it.each(["unraid", "unknown", "compose", "self", "protected"] as const)("goal 1/5: %s container shows no update or policy", async (origin) => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const locked = { ...container, ...(origin === "protected" ? { redeployDisabled: true } : origin === "self" ? { dockerLabels: { "io.docklet.system": "agent" } } : { origin }) } as ContainerInfo;
  const select = vi.fn(); renderWithApp(<ContainerUpdates c={locked} select={select} />, f.adapter);
  await act(async () => { await Promise.resolve(); });
  expect(screen.queryByRole("combobox") === null).toBe(true);
  expect(screen.queryByRole("button", { name: catalogs.de.common.updates.actionUpdateContainer }) === null).toBe(true);
  expect(f.calls.some((c) => c.path.includes("/policies/") || c.path.endsWith("/previous-image"))).toBe(false);
});

it("goal 1: missing scheduled target is removed with PUT mode=check", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const p = { ...storedPolicy, mode: "scheduled" as const, targetMissing: true, window: { kind: "daily" as const, time: "04:00" } };
  renderWithApp(<PolicyControl policy={p} timeZone="UTC" saved={() => {}} />, f.adapter);
  const remove = await screen.findByRole("button", { name: catalogs.de.settings.updates.remove });
  await waitFor(() => expect(remove.hasAttribute("disabled")).toBe(false)); await userEvent.click(remove);
  await waitFor(() => expect(f.writes()).toHaveLength(1));
  expect(f.writes()[0]!.init.method).toBe("PUT"); expect(JSON.parse(String(f.writes()[0]!.init.body))).toEqual({ mode: "check", expectedRevision: p.revision });
});

it("goal 1: scheduled mode cannot borrow check authority", async () => {
  const f = updateFixture(); f.state.denied.add("setAutoUpdateApply"); await f.adapter.getCurrentUser();
  renderWithApp(<PolicyControl policy={storedPolicy} timeZone="UTC" saved={() => {}} />, f.adapter);
  const trigger = screen.getByRole("combobox"); await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
  await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false)); trigger.focus(); await userEvent.keyboard("{Enter}");
  const scheduled = await screen.findByRole("option", { name: catalogs.de.settings.updates.scheduled });
  expect(scheduled.getAttribute("aria-disabled")).toBe("true"); expect(f.writes()).toHaveLength(0);
});

it.each(["update.service-coupled", "update.image-binding-unsupported"])("goal 4: service selection and %s whole-stack hint", async (code) => {
  const f = updateFixture(); f.state.code = code; await f.adapter.getCurrentUser();
  renderWithApp(<StackUpdateDialog stackId="direct~project" services={["app"]} onClose={() => {}} />, f.adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false));
  await userEvent.click(send); await screen.findByRole("alert");
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toMatchObject({ services: ["app"] });
  expect(screen.queryByText(t.wholeStack) !== null).toBe(true);
  const select = screen.getByRole("combobox"); await waitFor(() => expect(select.hasAttribute("disabled")).toBe(false)); select.focus(); await userEvent.keyboard("{Enter}"); await userEvent.click(await screen.findByRole("option", { name: t.update }));
  f.state.code = ""; await userEvent.click(send); await screen.findByText(t.accepted);
  expect(JSON.parse(String(f.writes()[1]!.init.body))).not.toHaveProperty("services");
});

function PreviousImageReadProbe() {
  const query = useQuery({ queryKey: ["container", container.id, "previous-image"], enabled: false });
  return <output data-testid="previous-read">{query.fetchStatus === "idle" && query.status !== "pending" ? `settled:${query.status}` : "pending"}</output>;
}
it.each([false, true])("goal 5: previous-image action follows Details.previousImage only (offered=%s)", async (offered) => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  f.adapter.getContainerPreviousImage = async () => offered ? { imageId: `sha256:${"b".repeat(64)}`, reference: "example:0.9.0", createdAt: point.createdAt } : undefined;
  f.adapter.getContainerUpdatePolicy = async () => ({ ...storedPolicy, targetKind: "container", targetId: "example", targetName: "example" });
  const done = vi.fn(f.adapter.getContainerPreviousImage); f.adapter.getContainerPreviousImage = done;
  renderWithApp(<><ContainerUpdates c={container} select={() => {}} /><PreviousImageReadProbe /></>, f.adapter);
  await waitFor(() => expect(done).toHaveBeenCalled());
  await waitFor(() => expect(screen.getByTestId("previous-read").textContent?.startsWith("settled:")).toBe(true));
  expect(screen.getByTestId("previous-read").textContent).toBe("settled:success");
  expect(screen.queryByRole("alert") === null).toBe(true);
  expect(screen.queryByRole("button", { name: t.previousImage }) !== null).toBe(offered);
});

it.each(["update.target-changed", "container.origin-locked", "image.restore-point-protected"])("goal 5: container restore refusal %s stays inline", async (code) => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  f.adapter.getContainerPreviousImage = async () => ({ imageId: `sha256:${"b".repeat(64)}`, reference: "example:0.9.0", createdAt: point.createdAt });
  f.state.code = code;
  renderWithApp(<ContainerUpdateDialog container={container} restore close={() => {}} />, f.adapter);
  const send = await screen.findByRole("button", { name: t.previousImage }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false));
  await userEvent.click(send); expect((await screen.findByRole("alert")).textContent).toContain((catalogs.de.images.errors as Record<string, string>)[code.replaceAll(".", "_")]);
  expect(f.writes()[0]!.path).toBe("/api/environments/direct/containers/example-id/previous-image/restore");
});

it("goal 5: standalone install uses durable container update route", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  Object.assign(f.run, { type: "container_redeploy", resourceType: "container", resourceId: "example-id" });
  await expect(f.adapter.installContainerUpdate(container.id)).resolves.toBe(f.run.id);
  expect(f.writes()[0]!.path).toBe("/api/environments/direct/containers/example-id/update");
  expect(new Headers(f.writes()[0]!.init.headers).get("Prefer")).toBe("respond-async");
  expect(new Headers(f.writes()[0]!.init.headers).has("Idempotency-Key")).toBe(true);
});

it.each(["project_deploy", "project_redeploy"] as const)("goal 6: failed %s recovery selects evidence point and keeps replacement consent", async (type) => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const run = renderRun(mapActivitySummary({ ...f.run, type, status: "failed", imageLifecycle: { runId: f.run.id, code: "runtime.interrupted", verified: false, volumeDataIncluded: false, restorePointId: point.id } }), "de");
  expect(hasDeployRecovery(run)).toBe(true);
  renderWithApp(<DeployRecovery run={run} />, f.adapter);
  const restore = await screen.findByRole("button", { name: t.previousDeployment }); await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(false)); await userEvent.click(restore);
  const dialog = within(await screen.findByRole("dialog")); const ack = await dialog.findByRole("checkbox"), send = dialog.getByRole("button", { name: t.restore });
  expect(send.hasAttribute("disabled")).toBe(true); await userEvent.click(ack); await userEvent.click(send); await dialog.findByText(t.accepted);
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toMatchObject({ pointId: point.id, replaceConfiguration: true });
});

it.each(["success", "running"] as const)("goal 6: %s deploy with point has no recovery gesture", async (status) => {
  const f = updateFixture(); const run = renderRun(mapActivitySummary({ ...f.run, type: "project_deploy", status, imageLifecycle: { runId: f.run.id, verified: false, volumeDataIncluded: false, restorePointId: point.id, pointCode: "deploy.point-unavailable" } }), "de");
  renderWithApp(<DeployRecovery run={run} />, f.adapter);
  expect(screen.queryByRole("button", { name: t.previousDeployment }) === null).toBe(true);
  expect(screen.queryByText(t.pointHint) !== null).toBe(true);
});

function PointProbe({ adapter }: { adapter: ReturnType<typeof updateFixture>["adapter"] }) {
  const q = useQuery({ queryKey: ["stack", "direct~project", "update-points"], queryFn: () => adapter.listRestorePoints!("direct~project") });
  return <output>{q.data?.[0]?.holdState}</output>;
}
it.each(["project_update", "project_restore", "project_deploy", "project_redeploy"])("RP10: terminal %s stream alone invalidates held points", async (type) => {
  const f = updateFixture(); await f.adapter.getCurrentUser(); f.state.points[0]!.holdState = "active";
  renderWithApp(<PointProbe adapter={f.adapter} />, f.adapter); await screen.findByText("active");
  const topics: string[][] = []; const off = f.adapter.subscribeChanges((value) => topics.push(value));
  const initialReads = f.calls.filter((c) => c.path.endsWith("/restore-points")).length;
  f.state.points[0]!.holdState = "none";
  await act(async () => { f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: point.createdAt, activity: { type: "updated", activity: { ...f.run, type, status: "success" } } }), lastEventId: "" } as MessageEvent); });
  expect(topics.some((value) => value.includes("stacks"))).toBe(true);
  off();
  await screen.findByText("none");
  expect(f.calls.filter((c) => c.path.endsWith("/restore-points"))).toHaveLength(initialReads + 1);
});

it("previous-image transport reads only the offered detail projection", async () => {
  const f = updateFixture();
  f.state.hold = (path) => path.endsWith("/containers/example-id") ? Promise.resolve(response({ success: true, data: { id: "example-id", previousImage: { imageId: `sha256:${"b".repeat(64)}`, imageRef: "example:0.9.0", createdAt: point.createdAt } } })) : undefined;
  expect(await f.adapter.getContainerPreviousImage!(container.id)).toMatchObject({ reference: "example:0.9.0" });
  f.state.hold = (path) => path.endsWith("/containers/example-id") ? Promise.resolve(response({ success: true, data: { id: "example-id" } })) : undefined;
  expect(await f.adapter.getContainerPreviousImage!(container.id)).toBeUndefined();
  expect(f.writes()).toHaveLength(0);
});


it("goal 1: protected self projection survives transport into the actual update consumer", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const raw = { id: "example-id", names: ["/example"], image: "example:1.0.0", state: "running", status: "Up", origin: "standalone", redeployDisabled: true } as unknown as ContainerRow;
  const mapped = mapContainer(raw, "direct", [], { stale: false });
  expect(mapped.redeployDisabled).toBe(true);
  renderWithApp(<ContainerUpdates c={{ ...mapped, update: container.update }} select={() => {}} />, f.adapter);
  expect(screen.queryByRole("button", { name: catalogs.de.common.updates.actionUpdateContainer }) === null).toBe(true);
  expect(screen.queryByRole("combobox") === null).toBe(true);
});


it("goal 5: an absent fresh slot disables a stale restore offer without a read error", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  f.state.hold = (path) => path.endsWith("/containers/example-id") ? Promise.resolve(response({ success: true, data: { id: "example-id" } })) : undefined;
  renderWithApp(<><ContainerUpdateDialog container={container} restore close={() => {}} /><PreviousImageReadProbe /></>, f.adapter);
  await waitFor(() => expect(screen.getByTestId("previous-read").textContent?.startsWith("settled:")).toBe(true));
  expect(screen.getByTestId("previous-read").textContent).toBe("settled:success");
  expect(screen.getByRole("button", { name: t.previousImage }).hasAttribute("disabled")).toBe(true);
  expect(screen.queryByRole("alert") === null).toBe(true); expect(f.writes()).toHaveLength(0);
});


it("goal 5: previous-image detail rejects a failed success envelope", async () => {
  const f = updateFixture();
  f.state.hold = (path) => path.endsWith("/containers/example-id") ? Promise.resolve(response({ success: false, data: { id: "example-id" } })) : undefined;
  await expect(f.adapter.getContainerPreviousImage!(container.id)).rejects.toHaveProperty("outcome.code", "http.error");
});
