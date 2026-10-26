// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original projection, delete-dialog and reconciliation gestures for issue #231.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { Link } from "react-router";
import { renderWithApp } from "@/test/render";
import { catalogs } from "@/app/i18n/catalog";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { response } from "@/data/http/runtime-commands.test-support";
import { deferred } from "@/data/http/activity-reader.test-support";
import { HostUpdateSettings } from "@/features/settings/sections/LiveUpdatesSection";
import { StackUpdateDialog } from "./stack-update-dialog";
import { LiveRestorePoints } from "./live-stack-updates";

const t = catalogs.de.common.stackUpdates, p = catalogs.de.common.restorePoints;
async function ready() { const f = updateFixture(); await f.adapter.getCurrentUser(); return f; }
async function openDelete() {
  const trigger = await screen.findByRole("button", { name: t.deletePoint });
  await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
  await userEvent.click(trigger);
  return within(screen.getByRole("dialog"));
}

it.each(["restore-target", "healthy-baseline"] as const)("unavailable %s anchor keeps its reason and is explicitly deletable", async (reason) => {
  const f = await ready(); f.state.points = [{ ...point, protectionReason: reason, failedAttempt: true, available: false }];
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  await screen.findByText(point.createdAt); expect(document.body.textContent).toContain(p.protection[reason]);
  expect(document.body.textContent).toContain(p.kind.failedAttempt);
  expect(screen.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(true);
  const dialog = await openDelete();
  expect(dialog.getByText(t.deleteHelp) !== null).toBe(true);
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint }));
  await screen.findByText(t.deleted); expect(f.writes()).toHaveLength(1);
});
it.each([false, true])("reload Direct/Edge=%s holds dominate null anchor and block restore/delete", async (edge) => {
  const f = await ready(); f.state.isEdge = edge; f.state.points = [{ ...point, holdState: "active" }];
  const first = renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  await screen.findByText(point.createdAt); expect(document.body.textContent).toContain(p.hold.active);
  expect(screen.queryByText(p.noRetentionAnchor) === null).toBe(true);
  expect(screen.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(true);
  first.unmount(); f.state.points[0]!.holdState = "unknown";
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  await screen.findByText(point.createdAt); expect(document.body.textContent).toContain(p.hold.unknown);
  expect(screen.queryByText(p.noRetentionAnchor) === null).toBe(true);
  expect(screen.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true);
  expect(f.writes()).toHaveLength(0);
});
it("hold wins the first label while the independent anchor remains visible", async () => {
  const f = await ready(); f.state.points = [{ ...point, protectionReason: "restore-target", holdState: "unknown" }];
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); await screen.findByText(p.hold.unknown);
  const article = screen.getByText(point.createdAt).closest("article")!;
  expect(article.textContent!.indexOf(p.hold.unknown)).toBeLessThan(article.textContent!.indexOf(p.protection["restore-target"]));
});
it("restore confirmation cannot reuse a point with uncertain hold", async () => {
  const f = await ready(); renderWithApp(<StackUpdateDialog stackId="direct~project" point={{ ...point, holdState: "unknown" }} onClose={() => {}} />, f.adapter);
  await userEvent.click(await screen.findByRole("checkbox"));
  expect(screen.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(true); expect(f.writes()).toHaveLength(0);
});
it.each(["protectionReason", "holdState", "failedAttempt", "images"])("bad %s fails the whole list without point controls", async (field) => {
  const f = await ready(); Object.assign(f.state.points[0]!, { [field]: field === "images" ? null : "future-value" });
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  expect((await screen.findByRole("alert")).textContent).toBe(catalogs.de.images.errors["update_check-unavailable"]);
  expect(screen.queryByRole("button", { name: t.deletePoint }) === null).toBe(true);
  expect(document.body.textContent).not.toContain("future-value"); expect(f.writes()).toHaveLength(0);
});
it.each(["listRestorePoints", "deleteRestorePoint"])("point UI intersects %s instead of route presence", async (missing) => {
  const f = await ready(); f.state.denied.add(missing); renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  if (missing === "listRestorePoints") { await screen.findByText(catalogs.de.activity.capability.unavailable); expect(screen.queryByText(point.createdAt) === null).toBe(true); }
  else { await screen.findByText(point.createdAt); expect(screen.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true); }
  expect(f.writes()).toHaveLength(0);
});
it("cancel and close send zero deletes; deliberate confirmation sends exactly one", async () => {
  const f = await ready(); renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  let dialog = await openDelete(); await userEvent.click(dialog.getByRole("button", { name: t.close }));
  expect(f.writes()).toHaveLength(0); dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByText(t.deleted);
  expect(f.writes()).toHaveLength(1); expect(screen.queryByRole("dialog") !== null).toBe(true);
});
it.each(["restore.in-use", "restore.not-found", "image.busy", "update.persistence-failed", "unexpected.raw-code"])("delete refusal %s stays inline", async (code) => {
  const f = await ready(), started = deferred<void>(), reply = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return reply.promise; } };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  const dialog = await openDelete();
  if (code === "restore.in-use") f.state.points[0]!.holdState = "active";
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  await act(async () => { reply.resolve(response({ success: false, error: { code, message: "raw diagnostic" } }, 409)); });
  expect(dialog.queryByRole("alert")?.textContent).toBe((catalogs.de.images.errors as Record<string, string>)[code.replaceAll(".", "_")] ?? catalogs.de.common.commandErrors.failed);
  expect(screen.queryByRole("dialog") !== null).toBe(true); expect(document.body.textContent).not.toContain("raw diagnostic");
  if (code === "restore.in-use") { await within(screen.getByRole("dialog")).findByText(p.hold.active); expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true); }
  expect(f.writes()).toHaveLength(1);
});
it("delete permission loss blocks repeated confirmation", async () => {
  const f = await ready(); f.state.code = "update.permission-denied"; f.state.status = 403;
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByRole("alert");
  expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("unknown delete reconciles actor B's deletion without replay", async () => {
  const f = await ready(), listed = deferred<void>();
  f.state.hold = (path, init) => { if (init.method === "DELETE") { f.state.points = []; return Promise.reject(new TypeError("lost")); } if (path.endsWith("/restore-points") && f.writes().length) listed.resolve(); };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await listed.promise;
  await act(async () => { await Promise.resolve(); }); expect(dialog.queryByText(t.alreadyDeleted) !== null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it.each([false, true])("unknown delete and unreadable list remain unknown until an authoritative refresh host=%s", async (host) => {
  const f = await ready(); f.state.points[0]!.orphaned = host; let unreadable = false;
  f.state.hold = (path, init) => { if (init.method === "DELETE") { unreadable = true; return Promise.reject(new TypeError("lost")); } if (path.endsWith("/restore-points") && unreadable) return Promise.resolve(response({ code: "agent.unavailable" }, 502)); };
  renderWithApp(host ? <HostUpdateSettings hostId="direct" name="Example" /> : <LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await act(async () => { await Promise.resolve(); });
  expect(dialog.queryByRole("alert") !== null).toBe(true);
  expect(document.body.textContent).toContain(t.deleteUnknown); expect(dialog.queryByText(t.deleteStillPresent) === null).toBe(true); expect(screen.queryByText(t.deleted) === null).toBe(true);
  expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true);
  unreadable = false; await userEvent.click(dialog.getByRole("button", { name: t.refreshPoints })); await screen.findByText(t.deleteStillPresent);
  await waitFor(() => expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(false));
  expect(f.writes()).toHaveLength(1);
  // Another actor wins after the authoritative refresh but before the explicit retry.
  f.state.hold = undefined; f.state.code = "restore.not-found";
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await act(async () => { await Promise.resolve(); }); expect(dialog.queryByText(t.alreadyDeleted) !== null).toBe(true);
  expect(f.writes()).toHaveLength(2);
});
it("a malformed successful delete is not presented as deleted", async () => {
  const f = await ready(); f.state.hold = (_path, init) => init.method === "DELETE" ? Promise.resolve(response({ success: true, data: {} })) : undefined;
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByText(t.deleteStillPresent);
  expect(document.body.textContent).toContain(t.deleteUnknown); expect(screen.queryByText(t.deleted) === null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("a hold refusal after an uncertain delete still appears inline", async () => {
  const f = await ready(); let lost = true;
  f.state.hold = (_path, init) => init.method === "DELETE" && lost ? Promise.reject(new TypeError("lost")) : undefined;
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByText(t.deleteStillPresent);
  await waitFor(() => expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(false));
  lost = false; f.state.code = "restore.in-use"; f.state.points[0]!.holdState = "unknown";
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint }));
  await act(async () => { await Promise.resolve(); });
  expect(dialog.queryByRole("alert")?.textContent).toBe(catalogs.de.images.errors["restore_in-use"]);
  await dialog.findByText(p.hold.unknown); expect(f.writes()).toHaveLength(2);
});
it("an owner change during unknown-delete reconciliation clears the old confirmation", async () => {
  const f = await ready();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { f.state.points[0]!.targetIdentity = "c".repeat(64); return Promise.reject(new TypeError("lost")); } };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint }));
  await act(async () => { await Promise.resolve(); }); expect(screen.queryByRole("dialog") === null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("a held confirmation dispatches one delete while a second click is ignored", async () => {
  const f = await ready(), started = deferred<void>(), held = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true);
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); expect(f.writes()).toHaveLength(1);
  await act(async () => { held.resolve(response({ success: true, data: { message: "Deleted" } })); });
  await screen.findByText(t.deleted);
});
it("identity loss closes a held delete and ignores its late acknowledgment", async () => {
  const f = await ready(), started = deferred<void>(), held = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  await act(async () => { await f.adapter.logout(); });
  expect(screen.queryByRole("dialog") === null).toBe(true);
  await act(async () => { held.resolve(response({ success: true, data: { message: "Deleted" } })); });
  expect(screen.queryByText(t.deleted) === null).toBe(true);
});
it("reload after a delete reconciles absence and never dispatches another delete", async () => {
  const f = await ready();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { f.state.points = []; return Promise.reject(new TypeError("lost")); } };
  const first = renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByText(t.alreadyDeleted);
  first.unmount(); renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter);
  await screen.findByText(t.noPoints); expect(f.writes()).toHaveLength(1);
});
it("host administrator can explicitly delete an orphan without a replacement target", async () => {
  const f = await ready(); f.state.points = [{ ...point, orphaned: true, available: false, protectionReason: "healthy-baseline" }];
  renderWithApp(<HostUpdateSettings hostId="direct" name="Example" />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await screen.findByText(t.deleted);
  expect(f.writes()[0]!.path).toBe(`/api/environments/direct/updater/restore-points/${point.id}`); expect(f.writes()).toHaveLength(1);
});
it.each(["listHostRestorePoints", "deleteHostRestorePoint"])("host orphan deletion requires %s", async (missing) => {
  const f = await ready(); f.state.denied.add(missing); f.state.points[0]!.orphaned = true;
  renderWithApp(<HostUpdateSettings hostId="direct" name="Example" />, f.adapter);
  if (missing === "listHostRestorePoints") { await screen.findByText(catalogs.de.activity.capability.unavailable); expect(screen.queryByRole("button", { name: t.deletePoint }) === null).toBe(true); }
  else { await screen.findByRole("button", { name: t.deletePoint }); expect(screen.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true); }
  expect(f.writes()).toHaveLength(0);
});
it("late delete from a closed dialog cannot settle its replacement", async () => {
  const f = await ready(), started = deferred<void>(), held = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  renderWithApp(<LiveRestorePoints stackId="direct~project" />, f.adapter); let dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  await userEvent.click(dialog.getByRole("button", { name: t.close })); dialog = await openDelete();
  await act(async () => { held.resolve(response({ success: true, data: { message: "Deleted" } })); });
  expect(dialog.queryByText(t.deleted) === null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("stack switch clears the selected delete intent and ignores its late refusal", async () => {
  const f = await ready(), started = deferred<void>(), held = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  function Switching() { const [host, setHost] = useState("direct"); return <><button onClick={() => setHost("other")}>Switch</button><LiveRestorePoints stackId={`${host}~project`} /></>; }
  renderWithApp(<Switching />, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  await act(async () => { screen.getByText("Switch").click(); });
  expect(screen.queryByRole("dialog") === null).toBe(true);
  await act(async () => { held.resolve(response({ code: "restore.in-use" }, 409)); });
  expect(screen.queryByRole("alert") === null).toBe(true); expect(f.writes()).toHaveLength(1);
});
it("navigation discards the delete confirmation before a late acknowledgment", async () => {
  const f = await ready(), started = deferred<void>(), held = deferred<Response>();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  const view = renderWithApp(<><Link to="/elsewhere">Navigate</Link><LiveRestorePoints stackId="direct~project" /></>, f.adapter); const dialog = await openDelete();
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); await started.promise;
  await act(async () => { await view.router.navigate("/elsewhere"); });
  await act(async () => { held.resolve(response({ success: true, data: { message: "Deleted" } })); });
  expect(screen.queryByRole("dialog") === null).toBe(true); expect(screen.queryByText(t.deleted) === null).toBe(true);
});

it.each([false, true])("a failed list after opening delete blocks the retained confirmation host=%s", async (host) => {
  const f = await ready(); f.state.points[0]!.orphaned = host;
  function Panel() {
    const client = useQueryClient();
    return <><button onClick={() => void client.invalidateQueries({ queryKey: [host ? "schedule" : "stack"] })}>Refresh evidence</button>{host ? <HostUpdateSettings hostId="direct" name="Example" /> : <LiveRestorePoints stackId="direct~project" />}</>;
  }
  renderWithApp(<Panel />, f.adapter); const dialog = await openDelete();
  Object.assign(f.state.points[0]!, { holdState: "future-value" });
  await act(async () => { screen.getByText("Refresh evidence").click(); });
  // The independent parent load error proves the refetch has reached its consumer.
  await waitFor(() => expect(screen.queryAllByRole("alert").length).toBeGreaterThan(0));
  expect(dialog.queryByRole("alert")?.textContent).toBe(catalogs.de.images.errors["update_check-unavailable"]);
  expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(true);
  await userEvent.click(dialog.getByRole("button", { name: t.deletePoint })); expect(f.writes()).toHaveLength(0);
  expect(dialog.queryByRole("button", { name: t.refreshPoints }) !== null).toBe(true);
  f.state.points[0]!.holdState = "none";
  await userEvent.click(dialog.getByRole("button", { name: t.refreshPoints }));
  await waitFor(() => expect(dialog.getByRole("button", { name: t.deletePoint }).hasAttribute("disabled")).toBe(false));
  expect(dialog.queryByRole("alert") === null).toBe(true); expect(f.writes()).toHaveLength(0);
});

it.each([false, true])("an authoritative disappearance during read recovery clears confirmation host=%s", async (host) => {
  const f = await ready(); f.state.points[0]!.orphaned = host; let refresh!: () => Promise<void>;
  function Panel() { const client = useQueryClient(); refresh = () => client.invalidateQueries({ queryKey: [host ? "schedule" : "stack"] }); return host ? <HostUpdateSettings hostId="direct" name="Example" /> : <LiveRestorePoints stackId="direct~project" />; }
  renderWithApp(<Panel />, f.adapter); const dialog = await openDelete();
  Object.assign(f.state.points[0]!, { holdState: "future-value" }); await act(async () => { await refresh(); });
  await waitFor(() => expect(screen.queryAllByRole("alert").length).toBeGreaterThan(0));
  f.state.points = []; await userEvent.click(dialog.getByRole("button", { name: t.refreshPoints }));
  await waitFor(() => expect(screen.queryAllByRole("alert").length).toBe(0));
  expect(screen.queryByRole("dialog") === null).toBe(true); expect(f.writes()).toHaveLength(0);
});
