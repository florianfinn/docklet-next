// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer-shaped HTTP fixtures and behavioural guards for issue #231.

import { formatDateTime } from "@/domain/format";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { StackUpdatePanel } from "./update-panel";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { renderWithApp } from "@/test/render";
import { catalogs } from "@/app/i18n/catalog";
import { UpdateRadarWidget } from "@/features/dashboard/widgets/UpdateRadarWidget";
import type { Stack } from "@/domain/model";
import { StackUpdateDialog, StackUpdateDialogController } from "./stack-update-dialog";
import { LiveRestorePoints, LiveStackUpdates } from "./live-stack-updates";
import { HostUpdateSettings } from "@/features/settings/sections/LiveUpdatesSection";
import { updateReasons } from "@/data/http/update-validation";
import { withRunGate } from "@/data/run-gate";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { deferred } from "@/data/http/activity-reader.test-support";
import { response } from "@/data/http/runtime-commands.test-support";
const t = catalogs.de.common.stackUpdates;
async function ready() { const f = updateFixture(); await f.adapter.getCurrentUser(); return f; }
it.each([...[...updateReasons].filter((code) => code !== "update.schedule-owner-unset"), "unexpected.raw-code"])("real submit keeps %s inline without raw message", async (code) => {
  const f = await ready(); f.state.code = code;
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false));
  await userEvent.click(send);
  const error = await screen.findByRole("alert"); const key = code.replaceAll(".", "_");
  expect(error.textContent).toContain((catalogs.de.images.errors as Record<string, string>)[key] ?? catalogs.de.common.commandErrors.failed);
  expect(screen.queryByRole("dialog") !== null).toBe(true); expect(document.body.textContent).not.toContain("raw diagnostic"); expect(f.writes()).toHaveLength(1);
});
it("unavailable descriptor disables send without any effect", async () => {
  const f = await ready(); f.state.denied.add("getUpdateBaseline");
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  await screen.findByText(catalogs.de.activity.capability.unavailable);
  expect(screen.getByRole("button", { name: t.update }).hasAttribute("disabled")).toBe(true); expect(f.writes()).toHaveLength(0);
});
it("restore does not require a current baseline, but requires explicit replacement consent", async () => {
  const f = await ready(); f.state.baseline.available = false; f.state.baseline.code = "update.baseline-unavailable";
  renderWithApp(<StackUpdateDialog stackId="direct~project" point={point} onClose={() => {}} />, f.adapter);
  const ack = await screen.findByRole("checkbox"), send = screen.getByRole("button", { name: t.restore });
  expect(send.hasAttribute("disabled")).toBe(true); await userEvent.click(ack);
  expect(send.hasAttribute("disabled")).toBe(false); await userEvent.click(send); await screen.findByText(t.accepted);
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toMatchObject({ replaceConfiguration: true, expectedRevision: f.state.baseline.savedRevision, pointId: point.id });
});
it("cold policy shows sending and separate stale/check/apply status after remount", async () => {
  const f = await ready(); const view = renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter);
  await screen.findByText(/Benachrichtigung läuft oder Ergebnis unbekannt/); expect(document.body.textContent).toContain(t.stale); expect(document.body.textContent).toContain(catalogs.de.images.errors["update_config-drift"]);
  view.unmount(); renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter);
  await screen.findByText(/Benachrichtigung läuft oder Ergebnis unbekannt/); expect(f.writes()).toHaveLength(0);
});
it("policy mode gesture and revision refusal remain inside open dialog", async () => {
  const f = await ready(); f.state.code = "update.policy-changed";
  renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter); await userEvent.click(await screen.findByRole("button", { name: t.editPolicy }));
  const dialog = screen.getByRole("dialog"); const trigger = within(dialog).getByRole("combobox"); trigger.focus(); await userEvent.keyboard("{Enter}{ArrowDown}{Enter}");
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  await screen.findByText(catalogs.de.images.errors["update_policy-changed"]); expect(screen.queryByRole("dialog") !== null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("schedule current run and master-off survive owner claim; save clears owner-unset", async () => {
  const f = await ready(); f.state.schedule.revision = 17;
  renderWithApp(<HostUpdateSettings hostId="direct" name="Example" />, f.adapter);
  await screen.findByText(catalogs.de.images.errors["update_schedule-owner-unset"]); expect(document.body.textContent).toContain(t.masterOff);
  await userEvent.click(screen.getByRole("button", { name: t.editSchedules }));
  const interval = within(screen.getByRole("dialog")).getByRole("textbox", { name: t.polling });
  await userEvent.clear(interval); await userEvent.type(interval, "0 1 * * * *");
  await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: t.save }));
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true)); await screen.findByText(/0 1 \* \* \* \*/);
  expect(screen.queryByText(catalogs.de.images.errors["update_schedule-owner-unset"]) === null).toBe(true);
  expect(document.body.textContent).toContain(t.masterOff); expect(document.body.textContent).toContain("1dd08eac");
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toMatchObject({ expectedRevision: 17, pollingInterval: "0 1 * * * *" });
});
it("point list exposes confirmed delete with server projection", async () => {
  const f = await ready(); renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  await screen.findByText(formatDateTime(point.createdAt)); expect(document.body.textContent).toContain(t.noData);
  expect(screen.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(false); expect(document.body.textContent).toContain(catalogs.de.common.restorePoints.noRetentionAnchor); expect(f.writes()).toHaveLength(0);
});
it("closing before a delayed refusal prevents stale result publication", async () => {
  const f = await ready(), held = deferred<Response>(), started = deferred<void>();
  f.state.hold = (path, init) => { if (path.endsWith("/update") && init.method === "POST") { started.resolve(); return held.promise; } };
  const view = renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false)); await userEvent.click(send); await started.promise;
  view.unmount(); await act(async () => { held.resolve(response({ success: false, error: { code: "update.config-drift", message: "raw diagnostic" } }, 409)); });
  expect(screen.queryByRole("alert") === null).toBe(true); expect(f.writes()).toHaveLength(1);
});

it("lost permission disables repeat submit and keeps the denial inline", async () => {
  const f = await ready(); f.state.code = "update.permission-denied"; f.state.status = 403;
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false));
  await userEvent.click(send); await screen.findByRole("alert");
  expect(send.hasAttribute("disabled")).toBe(true); expect(f.writes()).toHaveLength(1);
});
it.each(["update.policy-changed", "update.invalid-policy"])("schedule save %s stays inline", async (code) => {
  const f = await ready(); f.state.code = code;
  renderWithApp(<HostUpdateSettings hostId="direct" name="Example" />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.editSchedules }));
  const dialog = screen.getByRole("dialog"); await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  expect((await screen.findByRole("alert")).textContent).toContain((catalogs.de.images.errors as Record<string, string>)[code.replaceAll(".", "_")]);
  expect(screen.queryByRole("dialog") !== null).toBe(true);
});

it.each(["pending", "sending", "sent", "unconfigured", "failed", "not-requested"] as const)("notification %s is read from durable candidate state", async (state) => {
  const f = await ready(); f.state.policy.candidates[0]!.notificationState = state;
  renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter);
  await screen.findByRole("button", { name: t.editPolicy }); expect(document.body.textContent).toContain(t.notification[state]); expect(f.writes()).toHaveLength(0);
});

it("a late save from a closed dialog cannot close the replacement dialog", async () => {
  const f = await ready(), held = deferred<Response>(), started = deferred<void>();
  f.state.hold = (path, init) => { if (path.includes("/policies/") && init.method === "PUT") { started.resolve(); return held.promise; } };
  renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.editPolicy }));
  await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: t.save })); await started.promise;
  await userEvent.keyboard("{Escape}"); await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: t.editPolicy }));
  await act(async () => { held.resolve(response({ success: true, data: f.state.policy })); });
  expect(screen.queryByRole("dialog") !== null).toBe(true);
});

it("100 stack observation panels do not fan out scoped descriptor reads", async () => {
  const f = await ready();
  renderWithApp(<>{Array.from({ length: 100 }, (_, i) => <StackUpdatePanel key={i} services={[]} actions={{ onUpdate: () => {} }} />)}</>, f.adapter);
  await act(async () => { await Promise.resolve(); });
  expect(f.calls.filter((call) => call.path.endsWith("/capabilities"))).toHaveLength(0);
});
it("changing host during a held dialog admission does not publish to the new dialog", async () => {
  const f = await ready(), held = deferred<Response>(), started = deferred<void>();
  f.state.hold = (path, init) => { if (path.endsWith("/update") && init.method === "POST") { started.resolve(); return held.promise; } };
  function SwitchingDialog() { const [host, setHost] = useState("direct"); return <><button onClick={() => setHost("other")}>Switch host</button><StackUpdateDialog stackId={`${host}~project`} onClose={() => {}} /></>; }
  renderWithApp(<SwitchingDialog />, f.adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false)); await userEvent.click(send); await started.promise;
  // The test actor changes the same mounted consumer while the old POST ignores abort.
  await act(async () => { screen.getByText("Switch host").click(); });
  await waitFor(() => expect(screen.getByRole("button", { name: t.update }).hasAttribute("disabled")).toBe(false));
  await act(async () => { held.resolve(response({ success: true, data: { code: "run.started", activityId: f.run.id, activity: f.run } }, 202)); });
  expect(screen.queryByRole("alert") === null).toBe(true);
  expect(screen.queryByText(t.accepted) === null).toBe(true);
  expect(screen.getByRole("button", { name: t.update }).hasAttribute("disabled")).toBe(false);
});

it("a point from a replaced stack identity cannot be restored", async () => {
  const f = await ready();
  renderWithApp(<StackUpdateDialog stackId="direct~project" point={{ ...point, targetIdentity: "c".repeat(64) }} onClose={() => {}} />, f.adapter);
  await userEvent.click(await screen.findByRole("checkbox"));
  expect(screen.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(true); expect(f.writes()).toHaveLength(0);
});

it("the radar service observation opens a whole-stack update review", async () => {
  const f = await ready();
  const stack: Stack = { id: "direct~project", hostId: "direct", name: "Example", labels: [], autoUpdate: { mode: "check" }, services: [{ name: "app", image: { repository: "example", tag: "1.0.0" }, state: "running", health: "none", ports: [], networks: [], restartPolicy: "always", memLimitMb: 0, restarts24h: 0, update: { state: "version", current: { repository: "example", tag: "1.0.0" }, latest: { repository: "example", tag: "1.1.0", version: "1.1.0" }, level: "minor", checkedAt: point.createdAt, registry: "docker.io" } }] };
  renderWithApp(<><UpdateRadarWidget stacks={[stack]} hosts={new Map()} /><StackUpdateDialogController /></>, f.adapter);
  await userEvent.click(screen.getByRole("button", { name: /Example \/ app/ }));
  await userEvent.click(screen.getByRole("button", { name: catalogs.de.common.updates.actionUpdateStack }));
  const dialog = await screen.findByRole("dialog", { name: t.update });
  const send = within(dialog).getByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false)); await userEvent.click(send);
  await within(dialog).findByText(t.accepted);
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toEqual({ expectedRevision: f.state.baseline.savedRevision });
});

it("a racing restore refusal stays in the acknowledged dialog", async () => {
  const f = await ready(); f.state.code = "update.config-drift";
  renderWithApp(<StackUpdateDialog stackId="direct~project" point={point} onClose={() => {}} />, f.adapter);
  await userEvent.click(await screen.findByRole("checkbox"));
  await userEvent.click(screen.getByRole("button", { name: t.restore }));
  expect((await screen.findByRole("alert")).textContent).toContain(catalogs.de.images.errors["update_config-drift"]);
  expect(screen.queryByRole("dialog") !== null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("a conflicting local intent is shown inline without sending a second effect", async () => {
  const f = await ready(), adapter = withRunGate(f.adapter), held = deferred<Response>(), started = deferred<void>();
  f.state.hold = (path, init) => { if (path.endsWith("/update") && init.method === "POST") { started.resolve(); return held.promise; } };
  const first = adapter.installStackUpdates("direct~project", { expectedRevision: "c".repeat(64) }); await started.promise;
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, adapter);
  const send = await screen.findByRole("button", { name: t.update }); await waitFor(() => expect(send.hasAttribute("disabled")).toBe(false)); await userEvent.click(send);
  expect((await screen.findByRole("alert")).textContent).toContain(catalogs.de.activity.runtime.busy); expect(f.writes()).toHaveLength(1);
  held.resolve(response({ success: true, data: { code: "run.started", activityId: f.run.id, activity: f.run } }, 202)); await first;
});

it("an uncertain policy save stays inline after a fresh authoritative read", async () => {
  const f = await ready();
  f.state.hold = (path, init) => { if (path.includes("/policies/") && init.method === "PUT") { f.state.policy = { ...f.state.policy, mode: "off", revision: 2 }; return Promise.reject(new TypeError("network")); } };
  renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.editPolicy }));
  await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: t.save }));
  await screen.findByText(`${t.off} · ${t.stale}`);
  const alert = screen.queryByRole("alert"); expect(alert !== null).toBe(true);
  expect(alert?.textContent).toContain(catalogs.de.activity.runtime.ambiguous);
  expect(within(screen.getByRole("dialog")).getByRole("button", { name: t.save }).hasAttribute("disabled")).toBe(true);
  expect(f.writes()).toHaveLength(1);
});


it("initial missing baseline shows deploy-once guidance before any submit", async () => {
  const f = await ready(); f.state.baseline.available = false; f.state.baseline.code = "update.baseline-unavailable";
  const completed = deferred<void>(), original = f.adapter.getUpdateBaseline!;
  f.adapter.getUpdateBaseline = async (id) => { const value = await original(id); completed.resolve(); return value; };
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  await act(async () => { await completed.promise; });
  await waitFor(() => expect(screen.getByRole("button", { name: t.update }).hasAttribute("disabled")).toBe(false));
  const status = screen.queryByText(catalogs.de.images.errors["update_baseline-unavailable"]);
  expect(status?.getAttribute("role")).toBe("status");
  expect(screen.queryByRole("dialog") !== null).toBe(true); expect(f.writes()).toHaveLength(0);
});
it("readable baseline cannot enable send without the write descriptor", async () => {
  function ReadStatus() { const query = useQuery({ queryKey: ["stack", "direct~project", "update-baseline"], enabled: false }); return <span>{query.isSuccess ? "Baseline read settled" : "Baseline read pending"}</span>; }
  const f = await ready(); f.state.denied.add("installStackUpdates");
  const completed = deferred<void>(), original = f.adapter.getUpdateBaseline!;
  f.adapter.getUpdateBaseline = async (id) => { const value = await original(id); completed.resolve(); return value; };
  renderWithApp(<><StackUpdateDialog stackId="direct~project" onClose={() => {}} /><ReadStatus /></>, f.adapter);
  await act(async () => { await completed.promise; });
  await screen.findByText("Baseline read settled");
  await screen.findByText(catalogs.de.activity.capability.unavailable);
  const send = screen.getByRole("button", { name: t.update });
  expect(send.hasAttribute("disabled")).toBe(true); await userEvent.click(send); expect(f.writes()).toHaveLength(0);
});


it("HTTP header check is enabled only by the stack-scoped descriptor", async () => {
  const f = await ready(), original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    const response = await original(input, init), url = new URL(String(input), "https://hub.example.com");
    if (!url.pathname.endsWith("/capabilities")) return response;
    const body = await response.json();
    body.data = body.data.map((d: { operationId: string }) => d.operationId === "checkUpdates" ? { ...d, available: url.searchParams.get("kind") === "stack" && url.searchParams.get("id") === "direct~project" } : d);
    return new Response(JSON.stringify(body), { headers: response.headers });
  });
  renderWithApp(<StackHeader stack={{ id: "direct~project", hostId: "direct", name: "Example", labels: [], services: [], autoUpdate: { mode: "check" }, deployments: [], restorePoints: [], compose: "", env: "" }} host={undefined} />, f.adapter);
  await userEvent.click(screen.getByRole("button", { name: catalogs.de.stacks.detail.redeployMenu }));
  const item = await screen.findByRole("menuitem", { name: catalogs.de.stacks.detail.actions.check });
  await waitFor(() => expect(item.getAttribute("aria-disabled") === "true").toBe(false));
  await userEvent.click(item);
  await waitFor(() => expect(f.writes().some((write) => write.path === "/api/environments/direct/updater/projects/project/check")).toBe(true));
});
