// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original per-service deploy stage projection and behavioural coverage.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, describe, expect, it } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "@/data/AdapterProvider";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { activity, detail, type Activity } from "@/data/http/activity-reader.test-support";
import { mapActivityDetail, mapActivitySummary } from "@/data/http/map-dto";
import { envelope, response, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "@/data/http/runtime-commands.test-support";
import { renderRun, type CodedActionRun } from "@/data/message-code";
import { AdapterContext } from "@/data/use-adapter";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { ActionDialog } from "./ActionDialog";
import { actionDialogStore, openActionDialog } from "./action-dialog-store";
import { deployServicesOf, parseDeployServices, serviceSegments, serviceSummary, type DeployServiceRow } from "./deploy-services";
import { RunFocusBody } from "./RunFocus";

afterEach(() => { actionDialogStore.reset(); uiStyleStore.reset(); localStorage.clear(); });

const services = { v: 1, services: [
  { name: "app", check: true, pull: { layersDone: 1, layersTotal: 4, bytesDone: 41_234_567, bytesTotal: 312_456_789 } },
  { name: "db", check: false, create: "done", start: "done", health: "done" },
  { name: "web", build: "running" },
  { name: "worker", pull: { layersDone: 2, layersTotal: 2 }, create: "done", start: "error" },
] };
const row = (status: Activity["status"] = "running", step = "pull", deployServices: unknown = services): Activity => ({ ...activity("good"), status, step, metadata: { deployServices } });
const states = (r: DeployServiceRow, status: "running" | "succeeded" | "failed", prepared = false) => serviceSegments(r, { status }, prepared).map((s) => s.state).join(" ");

describe("parseDeployServices", () => {
  it("copies valid rows and hides the whole projection for any invalid row", () => {
    expect(parseDeployServices(services)?.rows.map((r) => r.name)).toEqual(["app", "db", "web", "worker"]);
    expect(parseDeployServices({ ...services, truncated: true })?.truncated).toBe(true);
    for (const invalid of [undefined, null, [], { ...services, v: 2 }, { ...services, truncated: false }, { v: 1, services: [] },
      { v: 1, services: Array.from({ length: 65 }, (_, i) => ({ name: `svc-${i}` })) },
      { v: 1, services: [{ name: "" }] }, { v: 1, services: [{ name: "x".repeat(257) }] }, { v: 1, services: [{ name: "a", check: "yes" }] },
      { v: 1, services: [{ name: "a", start: "skipped" }] }, { v: 1, services: [{ name: "a", pull: { layersDone: 3, layersTotal: 2 } }] },
      { v: 1, services: [{ name: "a", pull: { layersDone: 0, layersTotal: 0 } }] }, { v: 1, services: [{ name: "a", pull: { layersDone: 1.5, layersTotal: 2 } }] }]) {
      expect(parseDeployServices(invalid)).toBeUndefined();
    }
    expect(parseDeployServices({ v: 1, services: [{ name: "a", pull: { layersDone: 1, layersTotal: 2, bytesDone: 5, bytesTotal: 4 } }] })?.rows[0]?.pull).toEqual({ layersDone: 1, layersTotal: 2 });
  });

  it("keeps rows after settlement but never for unknown outcomes or other run types", () => {
    const view = (activityRow: Activity) => (mapActivityDetail({ ...detail("good"), activity: activityRow }) as { deployServices?: unknown }).deployServices;
    expect(view(row("success", "deploy"))).toBeDefined();
    expect(view({ ...row(), type: "container_start" })).toBeUndefined();
    const settled = renderRun(mapActivitySummary(row("success", "deploy")), "en-US");
    expect(deployServicesOf(settled)?.prepared).toBe(true);
    expect(deployServicesOf({ ...settled, outcomeState: "unknown" })).toBeUndefined();
    expect((mapActivitySummary({ ...row(), metadata: undefined }) as { deployServices?: unknown }).deployServices).toBeUndefined();
  });
});

describe("serviceSegments", () => {
  it("derives pending, skipped and running stages from observations", () => {
    expect(states({ name: "a" }, "running")).toBe("pending pending pending pending pending");
    expect(states({ name: "a" }, "running", true)).toBe("skipped skipped pending pending pending");
    expect(states({ name: "a", start: "running" }, "running")).toBe("skipped skipped skipped running pending");
    expect(states({ name: "a", pull: { layersDone: 1, layersTotal: 2 } }, "running")).toBe("running pending pending pending pending");
    expect(states({ name: "a", pull: { layersDone: 1, layersTotal: 2 } }, "failed")).toBe("error pending pending pending pending");
    expect(states({ name: "a", check: false, start: "done", health: "done" }, "running")).toBe("skipped skipped skipped done skipped");
    expect(states({ name: "a", start: "done" }, "succeeded")).toBe("skipped skipped skipped done skipped");
    expect(serviceSegments({ name: "a", pull: { layersDone: 1, layersTotal: 4, bytesDone: 30, bytesTotal: 120 } }, { status: "running" }, false)[0]?.fraction).toBe(0.25);
  });

  it("names the end state without claiming health that Compose did not declare", () => {
    const summary = (r: DeployServiceRow, status: "running" | "succeeded" | "failed" = "running") => serviceSummary(r, serviceSegments(r, { status }, true), { status }).kind;
    expect(summary({ name: "a", check: true, start: "done", health: "done" })).toBe("healthy");
    expect(summary({ name: "a", start: "done", health: "done" })).toBe("ready");
    expect(summary({ name: "a", check: false, start: "done", health: "done" })).toBe("running");
    expect(summary({ name: "a", start: "done", health: "running" })).toBe("waiting");
    expect(summary({ name: "a", create: "error" })).toBe("failed");
    expect(summary({ name: "a" }, "failed")).toBe("notReached");
  });
});

function wrap(locale: "en-US" | "de", ui: ReactNode) {
  return render(<AdapterProvider adapter={createDemoAdapter()}><IntlProvider locale={locale} messages={catalogs[locale]}><MemoryRouter>{ui}</MemoryRouter></IntlProvider></AdapterProvider>);
}

it.each(["en-US", "de"] as const)("%s focus shows one row per service and the compact facts line", (locale) => {
  const run = renderRun(mapActivitySummary(row()), locale);
  const view = wrap(locale, <RunFocusBody run={run} now={Date.parse(run.startedAt) + 61_000} />);
  try {
    const list = view.container.querySelector('[data-slot="service-progress"]');
    expect(list?.querySelectorAll("[data-service]")).toHaveLength(4);
    const app = list?.querySelector('[data-service="app"]');
    expect(app?.getAttribute("data-summary")).toBe("pulling");
    expect(app?.textContent).toContain(locale === "de" ? "Download 13 %" : "Download 13%");
    expect(app?.textContent).toContain("39 MB / 298 MB");
    expect(app?.querySelector('[data-stage="pull"] [data-slot="service-fill"]')?.getAttribute("style")).toContain("width: 13%");
    expect(list?.querySelector('[data-service="db"] [data-stage="health"]')?.getAttribute("data-state")).toBe("skipped");
    expect(list?.querySelector('[data-service="web"]')?.getAttribute("data-summary")).toBe("building");
    expect(list?.querySelector('[data-service="worker"]')?.getAttribute("data-summary")).toBe("failed");
    const facts = view.container.querySelector('[data-slot="dialog-facts"]');
    expect(facts?.hasAttribute("data-compact")).toBe(true);
    expect(facts?.textContent).toBe("61 s · Example good");
    expect(view.container.querySelector('[data-fact="services"]') === null).toBe(true);
  } finally { view.unmount(); }
});

it("keeps the aggregate key figures when an older agent sends no rows", () => {
  const run = renderRun(mapActivitySummary({ ...row(), metadata: undefined }), "en-US");
  const view = wrap("en-US", <RunFocusBody run={run} now={0} />);
  try {
    expect(view.container.querySelector('[data-slot="service-progress"]') === null).toBe(true);
    expect(view.container.querySelector('[data-fact="services"]') !== null).toBe(true);
  } finally { view.unmount(); }
});

it.each(["timeline", "console"] as const)("%s dialog design renders the service rows", async (design) => {
  const f = runtimeFixture();
  f.state.run = snapshot("edge", { outcomeState: "known", type: "project_redeploy", resourceType: "stack", step: "deploy", metadata: { deployServices: services } } as Partial<ReturnType<typeof snapshot>>);
  f.state.read = () => envelope(runtimeDetail(f.state.run));
  f.state.activity = (url: URL) => url.pathname.endsWith("/activities") ? response({ success: true, data: [f.state.run], pagination: { totalItems: 1 } }) : envelope(runtimeDetail(f.state.run));
  const coded = mapActivitySummary(f.state.run as Activity);
  uiStyleStore.set((s) => ({ ...s, actionDialogDesign: design }));
  openActionDialog(RUN_ID);
  const adapter = { ...f.adapter, listActions: async () => [coded], subscribeActions: (push: (runs: CodedActionRun[]) => void) => { push([coded]); return () => {}; } };
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(<QueryClientProvider client={queryClient}><AdapterContext.Provider value={adapter}>
    <IntlProvider locale="en-US" messages={catalogs["en-US"]} timeZone="UTC"><TooltipProvider><MemoryRouter><ActionDialog /></MemoryRouter></TooltipProvider></IntlProvider>
  </AdapterContext.Provider></QueryClientProvider>);
  try {
    const dialog = await screen.findByRole("dialog");
    expect(dialog.querySelectorAll('[data-slot="service-progress"] [data-service]')).toHaveLength(4);
  } finally { view.unmount(); queryClient.clear(); f.helper.reset(); }
});
