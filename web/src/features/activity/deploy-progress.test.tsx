// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage. M37 #584: truthful activity titles, host targets and scan counts.

import type { ReactNode } from "react";
import { AdapterProvider } from "@/data/AdapterProvider";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { render as bareRender } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { createActivityReader } from "@/data/http/activity-reader";
import { activity, deferred, detail, type Activity, type Detail } from "@/data/http/activity-reader.test-support";
import { detailWithSummary, mapActivityDetail, mapActivitySummary } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { RunFacts, RunStatusLine } from "./RunFocus";
import { ActionToastView } from "./ActionToastView";
import type { DeployProgressView } from "./deploy-progress";

const pull = { v: 1, images: 1, layersDone: 1, layersTotal: 3, bytesDone: 120, bytesTotal: 300 };
const row = (step = "pull"): Activity => ({ ...activity("good"), step, metadata: { deployPull: pull, deployActivity: { v: 1, label: "start", services: 2 } } });
const full = (activity: Activity): Detail => ({ ...detail("good"), activity });
const projection = (run: object) => (run as DeployProgressView<object>).deployProgress;
function freeze<T>(value: T): T {
  if (value && typeof value === "object") { Object.values(value).forEach(freeze); Object.freeze(value); }
  return value;
}

it("copies validated progress from frozen rows and hides invalid or unsafe observations", () => {
  const input = freeze(full(row()));
  expect(projection(mapActivityDetail(input))).toEqual({ phase: "pull", pull: { images: 1, layersDone: 1, layersTotal: 3, bytesDone: 120, bytesTotal: 300 }, activity: { label: "start", services: 2 } });
  expect(projection(mapActivityDetail(full({ ...row(), metadata: undefined })))).toEqual({ phase: "pull" });
  for (const invalid of [null, [], "bad", { ...pull, v: 2 }, { ...pull, images: 65 }, { ...pull, layersTotal: 513 }, { ...pull, layersDone: 4 }, { ...pull, layersDone: -1 }, { ...pull, layersDone: 1.5 }]) {
    expect(projection(mapActivityDetail(full(freeze({ ...row(), metadata: { deployPull: invalid } }))))?.pull).toBeUndefined();
  }
  for (const bytes of [{ bytesDone: -1, bytesTotal: 300 }, { bytesDone: 400, bytesTotal: 300 }, { bytesDone: 0, bytesTotal: Number.MAX_SAFE_INTEGER + 1 }, { bytesDone: NaN, bytesTotal: 300 }, { bytesDone: 1.1, bytesTotal: 300 }]) {
    expect(projection(mapActivityDetail(full({ ...row(), metadata: { deployPull: { ...pull, ...bytes } } })))?.pull).toEqual({ images: 1, layersDone: 1, layersTotal: 3 });
  }
  for (const deployActivity of [{ v: 2, label: "start", services: 1 }, { v: 1, label: "done", services: 1 }, { v: 1, label: "start", services: 513 }, { v: 1, label: "start", services: 0 }]) {
    expect(projection(mapActivityDetail(full({ ...row(), metadata: { deployActivity } })))?.activity).toBeUndefined();
  }
  expect(input.activity.metadata?.deployPull).toEqual(pull);
  for (const status of ["success", "failed", "cancelled"] as const) expect(projection(mapActivityDetail(full({ ...row(), status })))).toBeUndefined();
});

it("summary authority never borrows a missing progress projection from detail", () => {
  const summary = freeze(mapActivitySummary({ ...row(), step: undefined, metadata: undefined }));
  const detailed = freeze(mapActivityDetail(full(row())));
  expect(projection(detailWithSummary(summary, detailed))).toBeUndefined();
  expect(projection(detailWithSummary(freeze(mapActivitySummary(row("deploy"))), detailed))?.phase).toBe("deploy");
});

it.each(["summary-first", "detail-first"] as const)("keeps status and progress atomic across %s and completion", async (order) => {
  let summary = freeze(row());
  let expanded = freeze(full(row("deploy")));
  const reads = {
    hosts: async () => ["good"], page: vi.fn(async () => ({ items: [summary], incomplete: false })),
    detail: vi.fn(async () => expanded), publish: vi.fn(), unauthorized: vi.fn(),
  };
  const reader = createActivityReader(reads);
  if (order === "summary-first") {
    await reader.load();
    const held = deferred<Detail>(); reads.detail.mockReturnValueOnce(held.promise);
    const staleDetail = reader.readDetail("run-good", "good");
    summary = freeze({ ...row("deploy"), metadata: undefined });
    await reader.load();
    held.resolve(expanded);
    expect(projection(await staleDetail)).toEqual({ phase: "deploy" });
    expect(projection(reader.runs()[0]!)).toEqual({ phase: "deploy" });
  } else {
    await reader.readDetail("run-good", "good");
    await reader.load();
    expect(projection(reader.runs()[0]!)).toMatchObject({ phase: "pull", pull: { bytesDone: 120 } });
    expanded = freeze(full({ ...row("deploy"), metadata: undefined }));
    await reader.readDetail("run-good", "good");
    expect(projection(reader.runs()[0]!)).toEqual({ phase: "deploy" });
  }
  const held = deferred<Detail>(); reads.detail.mockReturnValueOnce(held.promise);
  const late = reader.readDetail("run-good", "good");
  summary = freeze({ ...row(), status: "success" });
  await reader.load(); held.resolve(full(row()));
  const completed = await late;
  expect(completed.status).toBe("succeeded");
  expect(projection(completed)).toBeUndefined();
  expect(projection(reader.runs()[0]!)).toBeUndefined();
});

it.each(["pull", "deploy"])("renders the current %s observation in focus and all toast designs without a whole-run percent", (phase) => {
  const run = renderRun(mapActivitySummary(freeze(row(phase))), "en-US");
  const expected = phase === "pull" ? "Pull · 1/3 layers · 120 B / 300 B" : "Deploy · starting 2 services";
  const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter>
    <RunStatusLine run={run} /><RunFacts run={run} now={0} />
    {(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major onClose={() => {}} onOpen={() => {}} />)}
  </MemoryRouter></IntlProvider>);
  try {
    expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe(expected);
    for (const toast of view.container.querySelectorAll('[data-slot="action-toast"]')) expect(toast.textContent).toContain(expected);
    expect(view.container.textContent).not.toContain("0%");
  } finally { view.unmount(); }
});

it("translates reached phases and the nonfatal pull warning after completion", () => {
  const run = renderRun(mapActivityDetail({ ...full({ ...row(), status: "success" }),
    stepTrace: [{ id: "pull", name: "pull", status: "success", startedAt: row().startedAt }, { id: "deploy", name: "deploy", status: "success", startedAt: row().startedAt }],
    messages: [{ id: "warning", activityId: "run-good", message: "Preliminary pull failed; continuing deployment", level: "warning", step: "pull", code: "activity.message", createdAt: row().startedAt }],
  }), "de");
  expect(run.steps.map((step) => step.label)).toEqual(["Images abrufen", "Bereitstellen"]);
  expect(run.log[0]).toContain("wird fortgesetzt");
  expect(projection(run)).toBeUndefined();
});

it("keeps the older agent dash fallback in focus and every toast design", () => {
  const run = renderRun(mapActivitySummary(freeze({ ...row(), step: undefined, metadata: undefined })), "en-US");
  const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter>
    <RunFacts run={run} now={0} />
    {(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major onClose={() => {}} onOpen={() => {}} />)}
  </MemoryRouter></IntlProvider>);
  try {
    expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe("–");
    expect(view.container.textContent).not.toContain("0%");
    expect(view.container.textContent).not.toContain("0/0");
  } finally { view.unmount(); }
});

it("shows large pull byte counts in readable units", () => {
  const large: Activity = { ...activity("good"), step: "pull", metadata: { deployPull: { ...pull, bytesDone: 41_234_567, bytesTotal: 312_456_789 } } };
  const run = renderRun(mapActivitySummary(freeze(large)), "en-US");
  const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><MemoryRouter><RunFacts run={run} now={0} /></MemoryRouter></IntlProvider>);
  try {
    expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe("Pull · 1/3 layers · 39 MB / 298 MB");
  } finally { view.unmount(); }
});

function render(ui: ReactNode) { return bareRender(<AdapterProvider adapter={createDemoAdapter()}>{ui}</AdapterProvider>); }
