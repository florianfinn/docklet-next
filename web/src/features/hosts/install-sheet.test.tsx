// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original production sheet lifecycle and IPv6/Edge artifact tests.

import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapHost } from "@/data/http/hosts";
import { buildInstallArtifacts } from "@/domain/agent-install";
import { renderWithApp } from "@/test/render";
import { useState } from "react";
import InstallSheet from "./InstallSheet";
import { existingHostPlan, TOKEN_PLACEHOLDER } from "./existing-host-plan";
const host = mapHost({ id: "atlas", name: "Example", color: null, apiUrl: "http://[2001:db8::10]:3553", enabled: true, status: "online", isEdge: false });
const defaults = { projectDir: "/srv/projects", installDir: "/srv/agent", connection: "direct" as const, puid: "", pgid: "", detectSocketGroup: true };
const general = { hubName: "Example", hubAddress: "https://hub.example.com", domainUrl: "", linkBase: "auto" as const, timeZone: "UTC" };
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); localStorage.clear(); });
it.each(["direct", "edge"] as const)("H5 builds validated %s files without a credential mutation", connection => {
  const plan = existingHostPlan({ ...host, connection }, defaults, general);
  expect(plan.token).toBe(TOKEN_PLACEHOLDER);
  const files = buildInstallArtifacts(plan);
  expect(files.env).toContain(`AGENT_TOKEN='${TOKEN_PLACEHOLDER}'`);
  if (connection === "direct") { expect(plan.address).toBe("[2001:db8::10]"); expect(plan.port).toBe(3553); expect(files.compose).toContain('"[2001:db8::10]:3553:3552"'); }
  else { expect(files.env).toContain("EDGE_AGENT=true"); expect(files.compose).not.toContain('    ports:'); }
});
function Harness() { const [selected, setSelected] = useState<typeof host | null>(host); return <><button onClick={() => setSelected(host)}>Open files</button><button onClick={() => setSelected({ ...host, id: "boreal", name: "Other" })}>Switch host</button><InstallSheet host={selected} onClose={() => setSelected(null)} /></>; }
it("H5 opens read-only, warns before exactly one rotation and never persists the issued token", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  const rotate = vi.spyOn(adapter, "issueAgentToken").mockResolvedValue("example-issued-token");
  const storage = vi.spyOn(Storage.prototype, "setItem");
  renderWithApp(<Harness />, adapter);
  expect((await screen.findByTestId("agent-token")).textContent).toBe(TOKEN_PLACEHOLDER);
  expect(rotate).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "Generate new token" }));
  const confirm = await screen.findByRole("alertdialog");
  expect(confirm.textContent).toContain("running agent disconnects"); expect(rotate).not.toHaveBeenCalled();
  await userEvent.click(within(confirm).getByRole("button", { name: "Generate new token" }));
  await waitFor(() => expect(screen.getByTestId("agent-token").textContent).toBe("example-issued-token"));
  expect(rotate).toHaveBeenCalledExactlyOnceWith(host.id);
  expect(screen.queryByRole("button", { name: "Generate new token" }) === null).toBe(true);
  expect(JSON.stringify(storage.mock.calls)).not.toContain("example-issued-token");
  for (const browserStorage of [localStorage, sessionStorage]) {
    const values = Array.from({ length: browserStorage.length }, (_, index) => browserStorage.getItem(browserStorage.key(index)!));
    expect(JSON.stringify(values)).not.toContain("example-issued-token");
  }
  expect(window.location.href).not.toContain("example-issued-token");
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.click(screen.getByRole("button", { name: "Open files" }));
  expect((await screen.findByTestId("agent-token")).textContent).toBe(TOKEN_PLACEHOLDER);
  expect(rotate).toHaveBeenCalledTimes(1);
});
it("H5 clears a late issued token when navigation switches the host", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  let release!: (token: string) => void;
  const rotate = vi.spyOn(adapter, "issueAgentToken").mockImplementation(() => new Promise(resolve => { release = resolve; }));
  const { renderRoutes } = await import("@/test/render");
  const { act } = await import("@testing-library/react");
  const { router } = renderRoutes([{ path: "/", element: <InstallSheet host={host} onClose={() => {}} /> }, { path: "/other", element: <InstallSheet host={{ ...host, id: "boreal", name: "Other" }} onClose={() => {}} /> }], ["/"], adapter);
  await screen.findByTestId("agent-token");
  await userEvent.click(screen.getByRole("button", { name: "Generate new token" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Generate new token" }));
  await waitFor(() => expect(rotate).toHaveBeenCalledTimes(1));
  await act(async () => { await router.navigate("/other"); });
  await act(async () => { release("example-late-token"); });
  expect((await screen.findByTestId("agent-token")).textContent).toBe(TOKEN_PLACEHOLDER);
  expect(document.body.textContent).not.toContain("example-late-token");
  expect(rotate).toHaveBeenCalledTimes(1);
});
