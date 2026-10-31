// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original restart confirmation and runtime refusal gestures. M14 host maintenance and availability.

import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapHost } from "@/data/http/hosts";
import { runtimeResponseError } from "@/data/http/runtime-mapping";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { renderWithApp } from "@/test/render";
import { HostSettingsDialog } from "./HostSettingsDialog";
const host = mapHost({ id: "atlas", name: "Example", color: null, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });
it.each(["atlas", "0"])("H3 requires confirmation for remote restart and offers no restart for %s hub", async id => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  const restart = vi.spyOn(adapter, "restartAgent").mockResolvedValue("example-run");
  renderWithApp(<HostSettingsDialog host={{ ...host, id }} component={undefined} open onOpenChange={() => {}} />, adapter);
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  if (id === "0") { expect(screen.queryByRole("button", { name: "Restart agent" }) === null).toBe(true); return; }
  await userEvent.click(screen.getByRole("button", { name: "Restart agent" }));
  expect(restart).not.toHaveBeenCalled();
  const confirm = await screen.findByRole("alertdialog", { name: "Restart agent on Example?" });
  await userEvent.click(within(confirm).getByRole("button", { name: "Restart agent" }));
  await waitFor(() => expect(restart).toHaveBeenCalledExactlyOnceWith("atlas"));
});
function RuntimeAction() { const command = useContainerCommands(); return <button onClick={() => void command.run("example", "stop")}>Stop</button>; }
it("H3 shows host.restarting from the runtime producer at the actual command consumer", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  vi.spyOn(adapter, "runContainerCommand").mockRejectedValue(runtimeResponseError({ code: "host.restarting", status: 409 }, 409));
  const error = vi.spyOn(toast, "error").mockImplementation(() => "example");
  renderWithApp(<RuntimeAction />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Stop" }));
  expect(error).toHaveBeenCalledWith("The agent is restarting. Wait until it reconnects.");
});

it("H3 keeps typed admission failure in the active confirmation", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  vi.spyOn(adapter, "restartAgent").mockRejectedValue(runtimeResponseError({ code: "host.busy", status: 409 }, 409));
  renderWithApp(<HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} />, adapter);
  await userEvent.click(screen.getByRole("tab", { name: "Agent container" }));
  await userEvent.click(screen.getByRole("button", { name: "Restart agent" }));
  const confirm = await screen.findByRole("alertdialog");
  await userEvent.click(within(confirm).getByRole("button", { name: "Restart agent" }));
  expect((await within(confirm).findByRole("alert")).textContent).toBe("Runtime work is active on this host. Wait before restarting the agent.");
  expect(adapter.restartAgent).toHaveBeenCalledOnce();
});
it("H3 offers no runtime-run controls for an unknown agent restart outcome", async () => {
  setAppLocale("en-US");
  const { createHostRestarts } = await import("@/data/http/host-restarts");
  const { renderRun } = await import("@/data/message-code");
  const { RuntimeControls } = await import("@/features/activity/RuntimeControls");
  const at = "2026-10-31T00:00:00Z";
  const row = { runId: "example-run", createdAt: at, deadline: "2026-10-31T00:03:00Z", state: "unknown", settledAt: at, code: "agent.restart-unconfirmed" };
  const published: import("@/data/message-code").CodedActionRun[] = [];
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ success: true, data: row }), { status: 202 }));
  const restarts = createHostRestarts({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), generation: () => 0, publish: run => published.push(run), changed: () => {} });
  await restarts.restartAgent("atlas");
  const adapter = createDemoAdapter(); const runtime = vi.spyOn(adapter, "getRuntimeRun");
  renderWithApp(<RuntimeControls run={renderRun(published.at(-1)!, "en-US")} />, adapter);
  expect(screen.queryByRole("button", { name: "Refresh run" }) === null).toBe(true);
  expect(runtime).not.toHaveBeenCalled();
  restarts.reset();
});
