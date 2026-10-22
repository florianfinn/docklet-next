// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterContext } from "@/data/use-adapter";
import { useActionRuns } from "@/data/live";
import { renderRun } from "@/data/message-code";
import { runGateOf, withRunGate } from "@/data/run-gate";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { envelope, response, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "@/data/http/runtime-commands.test-support";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { useContainerMenu } from "@/features/containers/container-menu";
import type { ContainerInfo } from "@/domain/model";
import { ActionsMenu } from "@/shared/components/action-menu";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { RuntimeControls } from "./RuntimeControls";
import { RunStatusLine } from "./RunFocus";
import { ActionToastView } from "./ActionToastView";
import { toast } from "sonner";
import { ActionDialog } from "./ActionDialog";
import { actionDialogStore, openActionDialog } from "./action-dialog-store";
import { presentationStore } from "./run-presentation";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { deferred } from "@/data/http/activity-reader.test-support";
import { ActionToasts } from "./ActionToasts";

const clients: QueryClient[] = [];
afterEach(() => { clients.splice(0).forEach((client) => client.clear()); act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); }); vi.restoreAllMocks(); });
function mount(ui: ReactNode, adapter: DockletAdapter, locale: "en-US" | "de" = "en-US") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } }); clients.push(client);
  return render(<AdapterContext.Provider value={withRunGate(adapter)}><QueryClientProvider client={client}><IntlProvider locale={locale} messages={catalogs[locale]} timeZone="UTC"><MemoryRouter><TooltipProvider>{ui}</TooltipProvider></MemoryRouter></IntlProvider></QueryClientProvider></AdapterContext.Provider>);
}
function VisibleRun() { const run = useActionRuns()[0]; return run ? <RunStatusLine run={run} /> : null; }
function ContainerMenu({ container }: { container: ContainerInfo }) { return <ActionsMenu groups={useContainerMenu(container, { onOpen() {}, onEdit() {} }, false)} label="Example commands" />; }

it.each(["start", "stop", "restart", "redeploy"] as const)("existing stack %s button is keyboard-operable and accepted admission is never success", async (command) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { type: command === "redeploy" ? "project_redeploy" : "project_deploy", resourceType: "project", resourceId: "example-project" });
  const demo = createDemoAdapter(); const base = await demo.getStack("media"); const stack = { ...base, id: "edge~example-project", hostId: "edge", services: base.services.map((service) => ({ ...service, state: command === "start" ? "stopped" as const : "running" as const })) };
  const view = mount(<><StackHeader stack={stack as never} host={undefined} /><VisibleRun /></>, f.adapter);
  try {
    const names = { start: "Start", stop: "Stop", restart: "Restart", redeploy: "Pull & redeploy" };
    const button = screen.getByRole("button", { name: names[command] });
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false)); button.focus(); await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(f.commands()).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText("Execution unconfirmed") !== null).toBe(true));
    expect(screen.queryByText("Completed successfully")).toBeNull();
    const path = command === "start" ? "up" : command === "stop" ? "down" : command;
    expect(f.commands()[0]!.url.pathname).toBe(`/api/environments/edge/projects/example-project/${path}`);
  } finally { view.unmount(); }
});
it.each(["start", "stop", "restart"] as const)("existing container menu %s uses the single-container command route", async (command) => {
  const f = runtimeFixture(); f.state.run = snapshot("0");
  const base = (await createDemoAdapter().listContainers())[0]!;
  const container: ContainerInfo = { ...base, id: "0~example-container", hostId: "0", origin: "standalone", stackId: undefined, state: command === "start" ? "stopped" : "running" };
  const adapter = { ...f.adapter, listStacks: async () => [] };
  const view = mount(<ContainerMenu container={container} />, adapter);
  try {
    await userEvent.click(screen.getByRole("button", { name: "Example commands" }));
    const label = { start: "Start", stop: "Stop", restart: "Restart" }[command];
    const item = await screen.findByRole("menuitem", { name: label });
    await waitFor(() => expect(item.getAttribute("data-disabled") === null).toBe(true));
    item.focus(); await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(f.commands()).toHaveLength(1)); expect(f.commands()[0]!.url.pathname).toBe(`/api/environments/0/containers/example-container/${command}`);
  } finally { view.unmount(); }
});
it.each(["en-US", "de"] as const)("%s manual recovery requires reason plus explicit acknowledgment and never labels released unknown success", async (locale) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "executor-continuity";
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), locale);
  f.state.read = (_url, init) => init.method === "POST" ? envelope({ ...f.state.run, status: "failed" }) : envelope(runtimeDetail(f.state.run, { recovery: f.state.recovery }));
  const view = mount(<RuntimeControls run={run} />, f.adapter, locale);
  try {
    const button = screen.getByRole("button", { name: locale === "de" ? "Gesperrtes Ziel freigeben" : "Release blocked target" });
    expect(button.hasAttribute("disabled")).toBe(true);
    await userEvent.type(screen.getByRole("textbox"), "Executor stopped and state inspected"); expect(button.hasAttribute("disabled")).toBe(true);
    const ack = screen.getByRole("checkbox"); ack.focus(); await userEvent.keyboard(" ");
    expect(button.hasAttribute("disabled")).toBe(false); await userEvent.click(button);
    await waitFor(() => expect(f.calls.filter(({ url }) => url.pathname.endsWith("/release"))).toHaveLength(1));
    expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); }
});
it("ordinary user has no administrator recovery control; pending unknown has no loss alert in every toast design", async () => {
  const f = runtimeFixture(); f.state.admin = false; f.state.run = snapshot("edge");
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), "en-US");
  const view = mount(<><RuntimeControls run={run} />{(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />)}</>, f.adapter);
  try {
    expect(screen.queryByRole("button", { name: "Release blocked target" })).toBeNull();
    expect(screen.getAllByText("Execution unconfirmed").length).toBeGreaterThanOrEqual(3);
    expect(view.container.textContent).not.toContain("Authoritative executor evidence is unavailable");
    expect(view.container.textContent).not.toContain("Succeeded");
  } finally { view.unmount(); }
});
it("a current admission on the first observation offers its major dialog, then reconnect does not replay its failure", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { type: "project_redeploy", resourceType: "project", resourceId: "example" });
  const adapter = { ...f.adapter, getConnectedSettings: async () => ({ activity: { actionPresentation: "dialog" } }) as never };
  const shows = vi.spyOn(toast, "custom");
  const view = mount(<ActionToasts />, adapter);
  try {
    await act(async () => { await f.adapter.runStackCommand("edge~example", "redeploy"); });
    expect(actionDialogStore.get().runId).toBe(RUN_ID);
    f.state.run.status = "failed"; f.state.run.outcomeState = "known";
    await act(async () => { await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    const count = shows.mock.calls.length;
    await act(async () => { await f.adapter.listActions(); await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    expect(shows.mock.calls.length).toBe(count); expect(f.commands()).toHaveLength(1);
  } finally { view.unmount(); }
});

it.each(["stack", "container"] as const)("late %s command callback cannot toast or publish into a replacement session", async (kind) => {
  const f = runtimeFixture(); const errors = vi.spyOn(toast, "error"); const reached = deferred<void>(), release = deferred<void>();
  let settled: Promise<unknown> | undefined;
  f.state.command = async () => { reached.resolve(); await release.promise; return envelope({ code: "run.started", activityId: RUN_ID, activity: f.state.run }, 202); };
  function Command() {
    const stack = useStackCommands(), container = useContainerCommands();
    return <button onClick={() => { settled = (kind === "stack" ? stack : container).run("0~example", "start"); }}>Start example</button>;
  }
  const view = mount(<Command />, f.adapter);
  try {
    await userEvent.click(screen.getByRole("button", { name: "Start example" })); await reached.promise;
    await act(async () => { await f.adapter.logout(); release.resolve(); await settled; });
    expect(errors).not.toHaveBeenCalled(); expect(f.commands()).toHaveLength(1);
    expect(f.commands()[0]!.init.signal?.aborted).toBe(true);
  } finally { release.resolve(); view.unmount(); }
});

it("remote unconfirmed cancellation shows intent separately without claiming completion or outcome loss", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { cancelRequested: true });
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), "en-US");
  const view = mount(<RuntimeControls run={run} />, f.adapter);
  try {
    expect(view.container.textContent).toContain("Executor confirmation is pending");
    expect(view.container.textContent).toContain("Cancellation was requested");
    expect(view.container.textContent).not.toContain("The workflow has stopped");
    expect(view.container.textContent).not.toContain("Authoritative executor evidence is unavailable");
    expect(screen.getByRole("button", { name: "Request cancellation" }).hasAttribute("disabled")).toBe(true);
    expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); }
});

it.each([false, true])("actual ActionDialog runtime=%s has distinct sibling keys", async (runtime) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  const f = runtimeFixture();
  f.state.run = snapshot("edge", { ...(runtime ? {} : { outcomeState: undefined, cancelRequested: undefined }) });
  f.state.activity = (url) => url.pathname.endsWith("/activities")
    ? response({ success: true, data: url.pathname.includes("/edge/") ? [f.state.run] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } })
    : envelope(runtimeDetail(f.state.run));
  if (runtime) await f.adapter.getRuntimeRun!("edge", RUN_ID); else await f.adapter.listActions();
  const view = mount(<ActionDialog />, f.adapter);
  try {
    await act(async () => { openActionDialog(RUN_ID); });
    await screen.findByRole("dialog");
    expect(errors.mock.calls.filter((args) => args.some((value) => String(value).includes("same key")))).toEqual([]);
  } finally { view.unmount(); }
});
it("actual ActionDialog run switch resets reason acknowledgment and pending while ignoring the old callback", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.adapter.getRuntimeRun!("edge", RUN_ID);
  const other = "2dd08eac-6846-4ba1-bbea-a3e5112cc860"; f.state.run = snapshot("edge", { id: other }); await f.adapter.getRuntimeRun!("edge", other);
  f.state.read = (url) => envelope(runtimeDetail(snapshot("edge", { id: url.pathname.split("/").at(-1)! }), { recovery: f.state.recovery }));
  const pending = deferred<never>(); const release = vi.fn(() => pending.promise); const errors = vi.spyOn(console, "error").mockImplementation(() => {}); const notices = vi.spyOn(toast, "error");
  const view = mount(<ActionDialog />, { ...f.adapter, releaseRuntimeRun: release });
  try {
    await act(async () => { openActionDialog(RUN_ID); }); await screen.findByRole("dialog");
    await userEvent.type(screen.getByRole("textbox"), "Example inspected state"); await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Release blocked target" }));
    expect(release).toHaveBeenCalledTimes(1); expect(screen.getByRole("button", { name: "Refresh run" }).hasAttribute("disabled")).toBe(true);
    await act(async () => { openActionDialog(other); });
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe(""); expect(screen.getByRole("checkbox").getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("button", { name: "Refresh run" }).hasAttribute("disabled")).toBe(false);
    expect(screen.getByRole("button", { name: "Release blocked target" }).hasAttribute("disabled")).toBe(true);
    await act(async () => { pending.reject(new Error("Example old failure")); await pending.promise.catch(() => undefined); });
    expect(notices).not.toHaveBeenCalled(); expect(errors.mock.calls.filter((args) => args.some((value) => String(value).includes("same key")))).toEqual([]);
  } finally { view.unmount(); }
});
it.each(["en-US", "de"] as const)("%s terminal unknown without recovery detail uses neutral wording", async (locale) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge", { status: "failed" });
  const run = renderRun(await f.adapter.getRuntimeRun!("edge", RUN_ID), locale);
  const view = mount(<RuntimeControls run={run} />, f.adapter, locale);
  try {
    const text = view.container.textContent;
    expect(text).toContain(locale === "de" ? "sein Ausführungsergebnis ist nicht bestätigt" : "its execution outcome is not confirmed");
    expect(text).not.toContain(locale === "de" ? "steht aus" : "confirmation is pending");
    expect(text).not.toContain(locale === "de" ? "nicht wiederholen" : "Do not repeat");
    expect(screen.queryByRole("button", { name: locale === "de" ? "Gesperrtes Ziel freigeben" : "Release blocked target" })).toBeNull();
  } finally { view.unmount(); }
});

it.each(["en-US", "de"] as const)("%s actual dialog keeps the administrator recovery form after targeted reload failure", async (locale) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  await f.adapter.getRuntimeRun!("edge", RUN_ID);
  f.state.activity = (url) => url.pathname.endsWith("/activities")
    ? response({ success: true, data: url.pathname.includes("/edge/") ? [f.state.run] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } })
    : envelope(runtimeDetail(f.state.run));
  const view = mount(<ActionDialog />, f.adapter, locale);
  try {
    await act(async () => { openActionDialog(RUN_ID); }); await screen.findByRole("dialog");
    await userEvent.type(screen.getByRole("textbox"), "Executor inspected"); await userEvent.click(screen.getByRole("checkbox"));
    f.state.detailStatus = 503;
    await act(async () => { await f.adapter.listActions(); });
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("Executor inspected");
    expect(screen.getByRole("checkbox").getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("button", { name: locale === "de" ? "Gesperrtes Ziel freigeben" : "Release blocked target" }).hasAttribute("disabled")).toBe(false);
    expect(screen.queryByText(locale === "de" ? "Ausführung unbestätigt" : "Execution unconfirmed")).toBeNull(); expect(f.commands()).toHaveLength(0);
  } finally { view.unmount(); }
});

it.each(["503", "malformed"])("newer terminal list observation through %s releases the real gate and notifies once", async (fault) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  f.state.activity = (url) => url.pathname.endsWith("/activities")
    ? response({ success: true, data: url.pathname.includes("/edge/") ? [f.state.run] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } })
    : envelope(runtimeDetail(f.state.run));
  const adapter = withRunGate({ ...f.adapter, getConnectedSettings: async () => ({ activity: { actionPresentation: "toast" } }) as never });
  const shows = vi.spyOn(toast, "custom");
  const view = mount(<ActionToasts />, adapter);
  try {
    await act(async () => { await adapter.runContainerCommand("edge~target", "start"); await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    expect(runGateOf(adapter)!.size).toBe(1);
    toast.dismiss(RUN_ID); shows.mockClear();
    f.state.read = () => fault === "503" ? response({}, 503) : envelope(runtimeDetail(f.state.run, { recovery: undefined }));
    f.state.run = snapshot("edge", { status: "failed", outcomeState: "known" });
    await act(async () => { await adapter.listActions(); });
    expect(runGateOf(adapter)!.size).toBe(0);
    expect(shows.mock.calls.filter(([, options]) => options?.id === RUN_ID)).toHaveLength(1);
    await act(async () => { await adapter.listActions(); });
    expect(shows.mock.calls.filter(([, options]) => options?.id === RUN_ID)).toHaveLength(1);
    expect(f.commands()).toHaveLength(1);
  } finally { view.unmount(); }
});

it.each(["password", "login"])("same-user %s renewal cancels the old consumer intent and only a deliberate click rejoins work", async (renewal) => {
  const f = runtimeFixture(); const reached = deferred<void>(), release = deferred<void>();
  const errors = vi.spyOn(toast, "error"), successes = vi.spyOn(toast, "success");
  await f.adapter.getCurrentUser();
  let settled: Promise<unknown> | undefined;
  f.state.command = async () => { reached.resolve(); await release.promise; return envelope({ code: "run.started", activityId: RUN_ID, activity: f.state.run }, 202); };
  function Command() {
    const command = useContainerCommands();
    return <button onClick={() => { settled = command.run("0~target", "start"); }}>Start example</button>;
  }
  const view = mount(<Command />, f.adapter);
  try {
    await userEvent.click(screen.getByRole("button", { name: "Start example" })); await reached.promise;
    f.state.auth = (url) => url.pathname.endsWith("/auth/login")
      ? envelope({ user: { id: "example-user", username: "operator", requiresPasswordChange: false } }) : envelope({});
    await act(async () => {
      if (renewal === "password") await f.adapter.changePassword("example-password", "new-example-password");
      else await f.adapter.login("operator", "example-password");
      release.resolve(); await settled;
    });
    expect(errors).not.toHaveBeenCalled(); expect(successes).not.toHaveBeenCalled();
    expect(f.commands()).toHaveLength(1);
    expect(f.commands()[0]!.init.signal?.aborted).toBe(true);
    f.state.command = () => envelope({ code: "run.joined", activityId: RUN_ID, activity: f.state.run }, 200);
    await userEvent.click(screen.getByRole("button", { name: "Start example" })); await settled;
    expect(f.commands()).toHaveLength(2);
    expect(errors).not.toHaveBeenCalled(); expect(successes).not.toHaveBeenCalled();
  } finally { release.resolve(); view.unmount(); }
});

it("stale active generic snapshot cannot reopen a retained terminal run or replay its toast", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("edge");
  let stale: ReturnType<typeof snapshot> | undefined;
  const reached = deferred<void>(), release = deferred<void>();
  f.state.activity = async (url) => {
    if (url.pathname.endsWith("/activities")) return response({ success: true, data: url.pathname.includes("/edge/") ? [f.state.run] : [], pagination: { totalItems: url.pathname.includes("/edge/") ? 1 : 0 } });
    if (stale) { const captured = stale; reached.resolve(); await release.promise; return envelope(runtimeDetail(captured)); }
    return envelope(runtimeDetail(f.state.run));
  };
  const adapter = withRunGate({ ...f.adapter, getConnectedSettings: async () => ({ activity: { actionPresentation: "toast" } }) as never });
  const shows = vi.spyOn(toast, "custom"); const view = mount(<ActionToasts />, adapter);
  try {
    await act(async () => { await adapter.runContainerCommand("edge~target", "start"); await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    expect(runGateOf(adapter)!.size).toBe(1);
    stale = f.state.run;
    f.state.run = snapshot("edge", { status: "failed", outcomeState: "known" });
    await act(async () => { await f.adapter.getRuntimeRun!("edge", RUN_ID); });
    expect(runGateOf(adapter)!.size).toBe(0);
    const count = shows.mock.calls.filter(([, options]) => options?.id === RUN_ID).length;
    expect(count).toBeGreaterThan(0);
    const older = adapter.listActions(); await reached.promise;
    f.state.detailStatus = 503;
    let observed: Awaited<ReturnType<typeof adapter.listActions>> = [];
    await act(async () => { release.resolve(); observed = await older; });
    // Keep checking notification replay even when the stale-status assertion fails.
    expect.soft(observed[0], "stale generic row must keep the terminal run").toMatchObject({ status: "failed", outcomeState: "known" });
    stale = undefined; f.state.detailStatus = 200;
    await act(async () => { await adapter.listActions(); });
    expect(shows.mock.calls.filter(([, options]) => options?.id === RUN_ID), "stale generic row must not replay the terminal toast").toHaveLength(count);
    expect(runGateOf(adapter)!.size).toBe(0); expect(f.commands()).toHaveLength(1);
  } finally { release.resolve(); view.unmount(); }
});
