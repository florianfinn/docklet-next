// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original real hub refusal response and HTTP adapter consumer proof.

import { readFileSync } from "node:fs";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { renderRun, type CodedActionRun } from "@/data/message-code";
import { AdapterContext } from "@/data/use-adapter";
import { ActionDialog } from "@/features/activity/ActionDialog";
import { actionDialogStore, openActionDialog } from "@/features/activity/action-dialog-store";
import { ActionToastView } from "@/features/activity/ActionToastView";
import { runtimeState } from "@/features/activity/runtime-presentation";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { createHttpAdapter } from "./http-adapter";
import { mapActivityDetail, mapActivitySummary } from "./map-dto";
import { envelope, runtimeFixture } from "./runtime-commands.test-support";
import type { RuntimeDetail } from "./runtime-mapping";
import { replacementRefusal } from "./replacement-refusal.test-support";

// The Go transport test verifies the checked-in producer shape. Local evidence
// can additionally supply the exact live httptest response for both transports.
function realDetail(host: string): RuntimeDetail {
  const directory = process.env.DOCKLET_REPLACEMENT_UI_EVIDENCE;
  const wire = directory ? JSON.parse(readFileSync(`${directory}/${host}.json`, "utf8")) : structuredClone(replacementRefusal);
  wire.data.activity.environmentId = host;
  wire.data.activity.sourceEnvironmentId = host;
  return wire.data as RuntimeDetail;
}
afterEach(() => { actionDialogStore.reset(); uiStyleStore.reset(); localStorage.clear(); });

it.each(["direct", "edge"])("%s real hub refusal reaches activity/run consumers in EN and DE", async (host) => {
  const detail = realDetail(host);
  const activity = process.env.DOCKLET_REPLACEMENT_UI_EVIDENCE ? JSON.parse(readFileSync(`${process.env.DOCKLET_REPLACEMENT_UI_EVIDENCE}/${host}-activity.json`, "utf8")) : detail;
  const f = runtimeFixture(); f.state.read = () => envelope(detail);
  const adapter = createHttpAdapter({ fetch: f.fetcher as typeof fetch });
  try {
    const coded = await adapter.getRuntimeRun!(host, detail.activity.id);
    for (const locale of ["en-US", "de"] as const) {
      const expected = catalogs[locale].activity.runtime.busy;
      expect(runtimeState(renderRun(coded, locale))).toBe("busy");
      expect(renderRun(coded, locale).error).toBe(expected);
      expect(renderRun(mapActivityDetail(activity), locale).error).toBe(expected);
      const summary = mapActivitySummary(detail.activity);
      expect(runtimeState(renderRun(summary, locale))).toBe("busy");
      const run = renderRun(coded, locale);
      expect(run.log.some((line) => line.includes(locale === "de" ? "ungeprüft" : "unverified"))).toBe(false);
      uiStyleStore.set((s) => ({ ...s, actionDialogDesign: "console" }));
      openActionDialog(detail.activity.id);
      const displayed = { ...adapter, listActions: async () => [coded], subscribeActions: (push: (runs: CodedActionRun[]) => void) => { push([coded]); return () => {}; } };
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
      const view = render(<QueryClientProvider client={client}><AdapterContext.Provider value={displayed}>
        <IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><TooltipProvider><MemoryRouter>
          <ActionDialog />
          <ActionToastView run={run} design="bar" detail="full" now={Date.parse(run.finishedAt!)} major onClose={() => {}} onOpen={() => {}} />
        </MemoryRouter></TooltipProvider></IntlProvider>
      </AdapterContext.Provider></QueryClientProvider>);
      try {
        const dialog = await screen.findByRole("dialog");
        expect(within(dialog).getAllByText(expected).length).toBeGreaterThan(0);
        expect(dialog.textContent).not.toContain(locale === "de" ? "ungeprüft" : "unverified");
        const toast = view.container.querySelector('[data-slot="action-toast"]');
        expect(toast?.textContent).toContain(expected);
        expect(toast?.textContent).not.toContain(locale === "de" ? "ungeprüft" : "unverified");
      } finally { view.unmount(); client.clear(); actionDialogStore.reset(); }
    }
    // The same consumer must keep true interruptions and administrator releases.
    for (const disposition of ["interrupted", "released"] as const) {
      const control = structuredClone(detail);
      control.activity.messageCode = "runtime.interrupted";
      control.activity.error = "run.busy";
      control.activity.outcomeState = disposition === "released" ? "unknown" : "known";
      control.recovery.disposition = disposition;
      f.state.read = () => envelope(control);
      const run = await adapter.getRuntimeRun!(host, control.activity.id);
      expect(runtimeState(renderRun(run, "en-US"))).toBe(disposition);
    }
  } finally { f.helper.reset(); }
});

it.each(["en-US", "de"] as const)("%s cancelling gate refusal uses existing refusal text", (locale) => {
  const detail = realDetail("direct");
  detail.activity.messageCode = "run.cancelling";
  detail.activity.error = "run.cancelling";
  const summary = mapActivitySummary(detail.activity);
  expect(runtimeState(renderRun(summary, locale))).toBe("cancellingConflict");
  expect(renderRun(summary, locale).error).toBe(catalogs[locale].activity.runtime.cancellingConflict);
});
