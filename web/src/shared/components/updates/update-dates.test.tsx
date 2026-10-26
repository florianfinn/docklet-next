// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original localized date regressions for live update views (#231).

import { act, screen, waitFor } from "@testing-library/react";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { formatDateTime } from "@/domain/format";
import { deferred } from "@/data/http/activity-reader.test-support";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { HostUpdateSettings } from "@/features/settings/sections/LiveUpdatesSection";
import { renderWithApp } from "@/test/render";
import { LiveRestorePoints, LiveStackUpdates } from "./live-stack-updates";
import { StackUpdateDialog } from "./stack-update-dialog";
import { DeletePointDialog } from "./delete-point-dialog";

for (const locale of ["de", "en-US"] as const) {
  it(`formats check, apply, schedule and point timestamps in ${locale}`, async () => {
    setAppLocale(locale);
    const f = updateFixture(); await f.adapter.getCurrentUser();
    f.state.policy.checkedAt = "2026-10-25T09:00:00Z";
    f.state.policy.applyCheckedAt = "2026-10-25T10:00:00Z";
    f.state.schedule.jobs[0]!.nextRun = "2026-10-27T11:00:00Z";
    f.state.points[0]!.orphaned = true;
    const reads = ["getAutoUpdatePolicy", "getUpdateSchedules", "listRestorePoints", "listHostRestorePoints"] as const;
    const completed = reads.map(() => deferred<void>());
    reads.forEach((name, index) => { const original = f.adapter[name]!; Object.assign(f.adapter, { [name]: async (id: string) => { const value = await original(id); completed[index]!.resolve(); return value; } }); });
    const view = renderWithApp(<><LiveStackUpdates stackId="direct~project" /><LiveRestorePoints stackId="direct~project" /><HostUpdateSettings hostId="direct" name="Example" /></>, f.adapter);
    await act(async () => { await Promise.all(completed.map((read) => read.promise)); });
    await screen.findByRole("button", { name: catalogs[locale].common.stackUpdates.editPolicy });
    await screen.findByRole("button", { name: catalogs[locale].common.stackUpdates.editSchedules });
    await waitFor(() => expect(screen.queryAllByRole("button", { name: catalogs[locale].common.stackUpdates.deletePoint }).length).toBe(2));
    for (const at of [f.state.policy.checkedAt, f.state.policy.applyCheckedAt, f.state.schedule.jobs[0]!.nextRun]) {
      expect(document.body.textContent).toContain(formatDateTime(at));
      expect(document.body.textContent).not.toContain(at);
    }
    expect(document.querySelector("time")?.textContent).toBe(formatDateTime(point.createdAt));
    expect(document.querySelector("time")?.getAttribute("dateTime")).toBe(point.createdAt);
    expect(screen.queryByText((_, element) => element?.tagName === "P" && element.textContent?.includes(point.targetIdentity) === true)?.textContent).toContain(formatDateTime(point.createdAt));
    expect(document.body.textContent).not.toContain(point.createdAt);
    view.unmount();
  });
  it.each(["restore", "delete"] as const)(`formats the %s dialog point timestamp in ${locale}`, async (kind) => {
    setAppLocale(locale);
    const f = updateFixture(); await f.adapter.getCurrentUser();
    const completed = deferred<void>(), original = f.adapter.getUpdateBaseline!;
    f.adapter.getUpdateBaseline = async (id) => { const value = await original(id); completed.resolve(); return value; };
    renderWithApp(kind === "restore" ? <StackUpdateDialog stackId="direct~project" point={point} onClose={() => {}} /> : <DeletePointDialog scope={{ kind: "stack", id: "direct~project" }} point={point} refresh={async () => [point]} close={() => {}} />, f.adapter);
    if (kind === "restore") { await act(async () => { await completed.promise; }); await screen.findByRole("checkbox"); }
    expect(screen.getByRole("dialog").textContent).toContain(formatDateTime(point.createdAt));
    expect(document.body.textContent).not.toContain(point.createdAt);
  });
}
