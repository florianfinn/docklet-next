// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Failed runtime DTOs rendered through the actual dialog and toast consumers.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { envelope, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "@/data/http/runtime-commands.test-support";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderMessage, renderRun, type CodedActionRun } from "@/data/message-code";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { ActionDialog } from "./ActionDialog";
import { actionDialogStore, openActionDialog } from "./action-dialog-store";
import { ActionToastView } from "./ActionToastView";
import { RunStatusLine } from "./RunFocus";

const sourceReason = "Runtime workflow or output failed; effects unverified";
const diagnostic = "manifest for registry.example.com/app:1 not found: manifest unknown";
const at = "2026-10-22T12:00:00Z";
const reasons = {
  "en-US": "The workflow or its output failed. Docker effects are unverified. Inspect the output and actual state before another action.",
  de: "Der Ablauf oder seine Ausgabe ist fehlgeschlagen. Die Docker-Auswirkungen sind ungeprüft. Vor einer weiteren Aktion die Ausgabe und den tatsächlichen Zustand prüfen.",
};
afterEach(() => { actionDialogStore.reset(); uiStyleStore.reset(); localStorage.clear(); });

it.each(["en-US", "de"] as const)("%s failed runtime dialog localises the coded reason, uses a complete headline and shows executor output", async (locale) => {
  const f = runtimeFixture();
  f.state.run = snapshot("edge", { type: "project_deploy", resourceType: "stack", status: "failed", outcomeState: "known", messageCode: "runtime.failed", error: sourceReason, latestMessage: sourceReason, endedAt: at });
  const detail = runtimeDetail(f.state.run, {
    recovery: { executorState: "failed", executorResultCode: "runtime.failed", releaseAcknowledged: false },
    output: [{ timestamp: at, text: JSON.stringify({ error: diagnostic }), stream: "stderr", level: "error", outputSeq: 1 }],
  });
  f.state.read = () => envelope(detail);
  const coded = await f.adapter.getRuntimeRun!("edge", RUN_ID);
  const run = renderRun(coded, locale);
  // The ordinary activity feed uses the same code mapping as targeted runtime reads.
  expect(renderRun(mapActivityDetail(detail), locale).error).toBe(reasons[locale]);
  expect(run.log).toEqual([reasons[locale]]);
  uiStyleStore.set((s) => ({ ...s, actionDialogDesign: "console" }));
  openActionDialog(RUN_ID);
  const adapter = { ...f.adapter, listActions: async () => [coded], subscribeActions: (push: (runs: CodedActionRun[]) => void) => { push([coded]); return () => {}; } };
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(<QueryClientProvider client={queryClient}><AdapterContext.Provider value={adapter}>
    <IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><TooltipProvider><MemoryRouter>
      <ActionDialog />
      <ActionToastView run={run} design="bar" detail="full" now={Date.parse(at)} major onClose={() => {}} onOpen={() => {}} />
    </MemoryRouter></TooltipProvider></IntlProvider>
  </AdapterContext.Provider></QueryClientProvider>);
  try {
    const dialog = await screen.findByRole("dialog");
    const status = dialog.querySelector('[role="status"][data-tone="err"]');
    expect(status?.textContent).toBe(locale === "de" ? "Fehlgeschlagen" : "Failed");
    expect(within(dialog).queryAllByText(reasons[locale]).length).toBeGreaterThan(0);
    expect(dialog.textContent).not.toContain(sourceReason);
    expect(dialog.querySelector('[data-slot="run-output"] li[data-stream="stderr"]')?.textContent).toContain(diagnostic);
    expect(dialog.textContent).not.toContain(locale === "de" ? "Noch keine Ausgabe" : "No output yet");
    const toast = view.container.querySelector('[data-slot="action-toast"]');
    expect(toast?.textContent).toContain(reasons[locale]);
    expect(toast?.textContent).not.toContain(locale === "de" ? "Fehler bei" : "Error at");
    expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); queryClient.clear(); f.helper.reset(); }
});

it.each(["en-US", "de"] as const)("%s failed headline preserves a known step and falls back for an empty label", (locale) => {
  const detail = runtimeDetail(snapshot("0", { status: "failed", messageCode: "runtime.failed", error: sourceReason }), {
    stepTrace: [{ id: "example-step", name: "pull", status: "failed", startedAt: at }],
  });
  const run = renderRun(mapActivityDetail(detail), locale);
  const view = render(<IntlProvider locale={locale} messages={catalogs[locale]}><RunStatusLine run={run} /></IntlProvider>);
  expect(screen.getByRole("status").textContent).toBe(locale === "de" ? "Fehler bei pull" : "Error at pull");
  view.rerender(<IntlProvider locale={locale} messages={catalogs[locale]}><RunStatusLine run={{ ...run, steps: run.steps.map((step) => ({ ...step, label: " " })) }} /></IntlProvider>);
  expect(screen.getByRole("status").textContent).toBe(locale === "de" ? "Fehlgeschlagen" : "Failed");
  view.unmount();
});

it.each(["en-US", "de"] as const)("%s runtime reason rendering covers coded failure, interruption and cancellation", (locale) => {
  for (const [code, key] of [["runtime.failed", "failedBody"], ["runtime.interrupted", "interruptedBody"], ["runtime.cancelled", "cancelledBody"]] as const) {
    expect(renderMessage({ code, params: {}, sourceText: sourceReason }, locale)).toBe(catalogs[locale].activity.runtime[key]);
  }
  const detail = runtimeDetail(snapshot("0", { status: "failed", error: "example uncoded error" }));
  expect(renderRun(mapActivityDetail(detail), locale).error).toBe("example uncoded error");
});
