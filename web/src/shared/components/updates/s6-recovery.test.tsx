// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { createTranslator } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { mapActivityDetail } from "@/data/http/map-dto";
import fixture from "@/data/http/testdata/update-recovery.json";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { ActionDialog } from "@/features/activity/ActionDialog";
import { actionDialogStore, openActionDialog } from "@/features/activity/action-dialog-store";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { RunFocusBody } from "@/features/activity/RunFocus";
import { RECOVERY_OUTCOME_KEYS } from "@/data/wire-catalogue-update";
import type { components } from "@/api/generated/schema";

export function recoveryRun(index = 0) {
  const at = "2026-11-12T12:00:00Z";
  return mapActivityDetail({ activity: { id: "example-run", type: "project_update", environmentId: "direct", status: "failed", startedAt: at, createdAt: at, imageLifecycle: fixture[index] as components["schemas"]["UpdaterLifecycleResult"] }, messages: [], output: [], stepTrace: [], outputTruncated: false });
}
afterEach(() => { setAppLocale("de"); actionDialogStore.reset(); uiStyleStore.reset(); });
for (const locale of ["en-US", "de"] as const) {
  it.each(fixture.map((row, index) => ({ outcome: row.recovery.outcome, index })))(`W-1 ${locale} renders Go-produced $outcome through decoder and activity detail`, ({ outcome, index }) => {
    setAppLocale(locale);
    const t = createTranslator({ locale, messages: catalogs[locale] });
    const run = renderRun(recoveryRun(index), locale);
    const view = renderWithApp(<RunFocusBody run={run} now={0} />);
    const line = view.container.querySelector(`[data-update-recovery="${outcome}"]`);
    expect(line !== null).toBe(true);
    expect(line?.textContent).toContain(t(RECOVERY_OUTCOME_KEYS[outcome as keyof typeof RECOVERY_OUTCOME_KEYS]));
    expect(line?.textContent).toContain(t("common.stackUpdates.volumeNotRestored"));
    expect(line?.textContent).toContain(t(`images.errors.${fixture[index]!.recovery.code.replace(".", "_")}` as never));
    expect(screen.queryByText("English producer fallback") === null).toBe(true);
  });
}
it("rejects unknown recovery outcomes rather than claiming a known result", () => {
  const run = recoveryRun();
  expect(run.updateRecovery?.outcome).toBe("restored");
  const at = "2026-11-12T12:00:00Z";
  expect(() => mapActivityDetail({ activity: { id: "example", type: "project_update", environmentId: "direct", status: "failed", startedAt: at, createdAt: at, imageLifecycle: { ...fixture[0], recovery: { outcome: "future" } } as never }, messages: [], output: [], stepTrace: [], outputTruncated: false })).toThrow();
});


it.each(["focus", "timeline", "console"] as const)("W-1 recovery stays visible in the actual %s activity dialog", async (design) => {
  setAppLocale("de"); uiStyleStore.set((state) => ({ ...state, actionDialogDesign: design }));
  const run = recoveryRun(), adapter = createDemoAdapter();
  adapter.listActions = async () => [run];
  adapter.subscribeActions = (notify) => { notify([run]); return () => {}; };
  adapter.observeActionDetail = (_host, _id, notify) => { notify({ status: "ready", run }); return () => {}; };
  renderWithApp(<ActionDialog />, adapter);
  act(() => openActionDialog(run.id));
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect(dialog.querySelector('[data-update-recovery="restored"]') !== null).toBe(true));
  expect(dialog.textContent).toContain(catalogs.de.images.errors.recovery_restored);
  expect(dialog.textContent).toContain(catalogs.de.common.stackUpdates.volumeNotRestored);
});
