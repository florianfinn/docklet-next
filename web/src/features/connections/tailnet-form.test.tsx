// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderRoutes, renderWithApp } from "@/test/render";
import SettingsPage from "@/features/settings/SettingsPage";
import SetupPage from "@/features/setup/SetupPage";
import { restartSetup, patchSetup } from "@/features/setup/setup-draft";
import { TailnetConnection } from "./TailnetConnection";
import { connectedStatus, peerHost, tailnetFixture } from "./tailnet.test-support";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); restartSetup(); vi.restoreAllMocks(); });

it("never prefills read secrets and clears the credential after save", async () => {
  const f = tailnetFixture();
  // Even a producer regression must not make a secret readable in the form.
  Object.assign(f.state.settings, { clientSecret: "test-only", authKey: "example" });
  renderWithApp(<TailnetConnection />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Credential and policy" }));
  const input = screen.getByLabelText("Client secret") as HTMLInputElement;
  expect(input.value).toBe("");
  expect(document.body.textContent?.includes("test-only")).toBe(false);
  await userEvent.type(input, "test-only");
  await userEvent.click(screen.getByRole("button", { name: "Save credential" }));
  await screen.findByText("Credential saved. Connection status updates live.");
  expect((screen.getByLabelText("Client secret") as HTMLInputElement).value).toBe("");
  expect(f.writes[0]?.body).toEqual({ enabled: true, mode: "oauth", clientId: "example-client", clientSecret: "test-only" });
});

it("requires a reset confirmation naming the host count and local-only effect; disable needs none", async () => {
  // Status counts hosts only while enabled; the dialog counts tailnet-route hosts from the catalogue.
  const f = tailnetFixture({ ...connectedStatus(), hostCount: 0 });
  f.state.rows = ["a", "b", "c"].map((id) => ({ ...peerHost, id: `example-agent-${id}`, name: `Example agent ${id}` }));
  renderWithApp(<TailnetConnection />, f.adapter);
  await waitFor(() => expect(f.requests.includes("/api/environments")).toBe(true));
  await userEvent.click(await screen.findByRole("button", { name: "Reset…" }));
  expect(f.writes).toHaveLength(0);
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog.textContent).toContain("3 hosts use this route and must be re-added afterwards");
  expect(dialog.textContent).toContain("docklet-hub stays in the tailnet admin console");
  expect(dialog.textContent).toContain("saved credential is kept");
  await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await userEvent.click(screen.getByRole("button", { name: "Disconnect" }));
  await waitFor(() => expect(f.writes).toHaveLength(1));
  expect(f.writes[0]).toEqual({ path: "/api/settings/connections/tailnet", body: { enabled: false } });
  expect(screen.queryByRole("alertdialog") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Reset…" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Reset identity" }));
  await waitFor(() => expect(f.writes.some((write) => write.path === "/api/tailnet/reset")).toBe(true));
});

it("shows provisioning rejection separately from the retained connection", async () => {
  const f = tailnetFixture({ ...connectedStatus(), provisioning: { state: "rejected", providerStatus: 403 }, routeAvailable: false });
  renderWithApp(<TailnetConnection />, f.adapter);
  // A rejected credential keeps the connection but must not promise the route to new hosts.
  await screen.findByText("Connected to example.com. New hosts cannot use this route until the problem below is solved.");
  expect(screen.getByText(/invalid or revoked/).textContent).toContain("Existing connected hosts keep working");
  expect(screen.getByText("Control server response: HTTP 403.") !== null).toBe(true);
  expect(screen.getByLabelText("Client secret") !== null).toBe(true);
});

it("copies the exact producer policy and keeps manual keys distinct", async () => {
  const user = userEvent.setup(); const copied = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
  const f = tailnetFixture({ ...connectedStatus(), enabled: false, connection: { state: "off" }, provisioning: { state: "not-configured" } });
  renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Copy policy" }));
  expect(copied).toHaveBeenCalledWith(f.state.settings.policySnippet);
  await user.click(screen.getByRole("radio", { name: /Manual keys/ }));
  expect(screen.queryByRole("link", { name: "Open admin console" }) === null).toBe(true);
  await user.type(screen.getByLabelText("Hub auth key"), "tskey-auth-example-hub");
  await user.click(screen.getByRole("button", { name: "Connect" }));
  await waitFor(() => expect(f.writes[0]?.body).toEqual({ enabled: true, mode: "manual", authKey: "tskey-auth-example-hub" }));
});

it.each(["off", "joining", "control-unreachable", "error"] as const)("renders an actionable response for %s", async (state) => {
  const f = tailnetFixture({ ...connectedStatus(), connection: { state } });
  renderWithApp(<TailnetConnection />, f.adapter);
  await screen.findByRole("button", { name: "Save credential" });
  expect(document.querySelector('[data-slot="tailnet-connection"] header [role="status"]')?.textContent?.length).toBeGreaterThan(20);
});

it("keeps Connections admin-only, including its grouped narrow-screen picker", async () => {
  const f = tailnetFixture();
  const view = renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/connections"], f.adapter);
  await screen.findByRole("button", { name: "Credential and policy" });
  const picker = screen.getByTestId("settings-section-picker"); picker.focus();
  await userEvent.keyboard("{Enter}");
  expect(within(await screen.findByRole("listbox")).getByRole("option", { name: /Connections/ }) !== null).toBe(true);
  view.unmount();
  const reader = tailnetFixture(connectedStatus(), false);
  renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/connections"], reader.adapter);
  await waitFor(() => expect(screen.queryByRole("link", { name: "Connections" }) === null).toBe(true));
  expect(reader.requests.some((path) => path.includes("/tailnet"))).toBe(false);
});

it("skips the optional setup step without a tailnet write", async () => {
  const f = tailnetFixture({ ...connectedStatus(), enabled: false, connection: { state: "off" } });
  patchSetup({ reached: 4 });
  const { router } = renderRoutes([{ path: "/setup", element: <SetupPage /> }], ["/setup?step=tailnet"], f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Skip" }));
  await waitFor(() => expect(router.state.location.search).toBe("?step=review"));
  expect(f.writes).toHaveLength(0);
});


it("shows an invalid or revoked manual key from the connection refusal", async () => {
  const f = tailnetFixture({ ...connectedStatus(), mode: "manual", connection: { state: "error", code: "tailnet.credential-rejected" }, provisioning: { state: "not-applicable" }, routeAvailable: false });
  renderWithApp(<TailnetConnection />, f.adapter);
  await screen.findByLabelText("Hub auth key");
  expect(screen.queryByText(/credential is invalid or revoked/) !== null).toBe(true);
  expect(screen.getByText("Credential rejected") !== null).toBe(true);
});


it("explains a control outage reported while the hub is still joining", async () => {
  const f = tailnetFixture({ ...connectedStatus(), connection: { state: "joining", code: "tailnet.coordination-unreachable" }, routeAvailable: false });
  renderWithApp(<TailnetConnection />, f.adapter);
  await screen.findByLabelText("Client secret");
  expect(screen.queryByText(/The control server cannot be reached/) !== null).toBe(true);
});

it("tells the operator when the provider cannot check the credential", async () => {
  const f = tailnetFixture({ ...connectedStatus(), provisioning: { state: "unavailable", code: "tailnet.provider-unavailable" } });
  renderWithApp(<TailnetConnection />, f.adapter);
  const line = await screen.findByText(/cannot check the credential right now/);
  expect(line.textContent).toContain("existing hosts stay connected");
});
