// Source: docklet-next/issues/586
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original Tailscale setup step and setup test proofs for M37 #586.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { deferred } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";

import { TailnetConnection } from "./TailnetConnection";
import { connectedStatus, PRODUCER_POLICY, tailnetFixture } from "./tailnet.test-support";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });

const csrf = { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") };
const allPass = (revision: string) => ({ credentialRevision: revision, stale: false, checks: ["credential", "agent-key", "cleanup", "hub-joined", "hub-tag", "hub-address"].map((id) => ({ id, state: "pass" })) });
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: csrf });
const checks = () => screen.queryByRole("list", { name: "4. Test the setup" });

it("orders the steps policy, credential, paste and test, with places and values apart from the prose", async () => {
  const f = tailnetFixture({ ...connectedStatus(), enabled: false, connection: { state: "off" }, provisioning: { state: "not-configured" } });
  f.state.settings = { ...f.state.settings, secretConfigured: false };
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  await screen.findByRole("button", { name: "Copy policy" });
  const titles = [...view.container.querySelectorAll('[data-slot="tailnet-step"] h4')].map((h) => h.textContent);
  expect(titles).toEqual(["1. Paste the policy", "2. Create the OAuth trust credential", "3. Paste and save"]);
  const places = [...view.container.querySelectorAll('[data-slot="step-where"]')].map((p) => p.textContent);
  expect(places).toEqual(["Tailscale admin console›Access controls›JSON editor".replaceAll("›", ""), "Settings›Trust credentials›New credential›OAuth›Custom scopes".replaceAll("›", "")]);
  expect(screen.getByRole("link", { name: "Open access controls" }).getAttribute("href")).toBe("https://login.tailscale.com/admin/acls/file");
  // Values to paste have their own copy buttons: scope and both tags.
  for (const value of ["Keys → Auth keys: write", "tag:docklet-hub", "tag:docklet-agent"]) expect(screen.getByRole("button", { name: new RegExp(`^Copy (Scope|Tag) ${value}$`) }) !== null).toBe(true);
  // Before a credential is saved, there is no test to run.
  expect(screen.queryByRole("button", { name: "Run test" }) === null).toBe(true);
});

it("offers the policy as form values for consoles without the JSON editor", async () => {
  const user = userEvent.setup(); const copied = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
  const f = tailnetFixture({ ...connectedStatus(), enabled: false, connection: { state: "off" }, provisioning: { state: "not-configured" } });
  expect(f.state.settings.policySnippet).toBe(PRODUCER_POLICY);
  renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "No JSON editor? Use the forms" }));
  for (const [label, value] of [["Tag name", "docklet-hub"], ["Tag name", "docklet-agent"], ["Tag owner", "autogroup:admin"], ["Source", "tag:docklet-agent"], ["Destination", "tag:docklet-hub"], ["Port and protocol", "tcp:3552"]] as const) {
    await user.click(screen.getByRole("button", { name: `Copy ${label} ${value}` }));
    expect(copied).toHaveBeenLastCalledWith(value);
  }
  expect(await screen.findByText("Port and protocol copied") !== null).toBe(true);
});

it("runs the setup test once a credential is saved and marks the setup verified only on a passing result", async () => {
  const user = userEvent.setup();
  const f = tailnetFixture();
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Run test" }));
  const list = await screen.findByRole("list", { name: "4. Test the setup" });
  expect(within(list).getAllByText("Passed").length).toBe(6);
  expect(within(list).getByText("Tagged agent key created") !== null).toBe(true);
  expect(view.container.textContent?.includes("Verified")).toBe(true);
  expect(f.writes.filter((w) => w.path === "/api/tailnet/test").length).toBe(1);
});

it("shows a failing check with its code and provider status, without the verified mark", async () => {
  const user = userEvent.setup();
  const f = tailnetFixture();
  const failing = allPass(f.state.status.credentialRevision!);
  failing.checks[1] = { id: "agent-key", state: "fail", code: "tailnet.mint-refused", providerStatus: 403 } as never;
  f.state.test = () => ok(failing);
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Run test" }));
  const list = await screen.findByRole("list", { name: "4. Test the setup" });
  expect(within(list).getByText("Failed") !== null).toBe(true);
  expect(within(list).getByText(/may not create agent keys/) !== null).toBe(true);
  expect(within(list).getByText("Control server response: HTTP 403.") !== null).toBe(true);
  expect(view.container.textContent?.includes("Verified")).toBe(false);
});

it("drops a result when the credential is replaced while the test runs, and hides an earlier result after a new save", async () => {
  const user = userEvent.setup();
  const f = tailnetFixture();
  const held = deferred<Response>();
  f.state.test = () => held.promise;
  renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Run test" }));
  await waitFor(() => expect(f.writes.some((w) => w.path === "/api/tailnet/test")).toBe(true));
  // A same-mode credential replaces the connected run while the test is held (generation kept by the hub).
  await user.click(screen.getByRole("button", { name: "Credential and policy" }));
  await user.type(screen.getByLabelText("Client secret"), "test-only-new");
  await user.click(screen.getByRole("button", { name: "Save credential" }));
  await waitFor(() => expect(f.writes.some((w) => w.path === "/api/settings/connections/tailnet")).toBe(true));
  await act(async () => { held.resolve(ok(allPass(f.state.status.credentialRevision!))); });
  await waitFor(() => expect(screen.queryByText("The credential changed during the test. Run it again.") !== null).toBe(true));
  expect(checks() === null).toBe(true);
  // A passing result is shown, then a later save hides it: it belongs to the replaced credential.
  f.state.test = undefined;
  await user.click(screen.getByRole("button", { name: "Run test" }));
  expect(await screen.findByRole("list", { name: "4. Test the setup" }) !== null).toBe(true);
  await user.type(screen.getByLabelText("Client secret"), "test-only-third");
  await user.click(screen.getByRole("button", { name: "Save credential" }));
  await waitFor(() => expect(checks() === null).toBe(true));
});

it("keeps the test button busy while a test runs and explains a test already running elsewhere", async () => {
  const user = userEvent.setup();
  const f = tailnetFixture();
  const held = deferred<Response>();
  f.state.test = () => held.promise;
  renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Run test" }));
  const busy = await screen.findByRole("button", { name: "Testing…" });
  expect(busy.hasAttribute("disabled")).toBe(true);
  await user.click(busy);
  expect(f.writes.filter((w) => w.path === "/api/tailnet/test").length).toBe(1);
  await act(async () => { held.resolve(new Response(JSON.stringify({ code: "tailnet.test-running", status: 409, detail: "running" }), { status: 409, headers: csrf })); });
  await waitFor(() => expect(screen.queryByText("A setup test is already running. Wait for it to finish.") !== null).toBe(true));
  expect(screen.getByRole("button", { name: "Run test" }).hasAttribute("disabled")).toBe(false);
});

it("hides a completed result once another credential revision is observed, and keeps it for observations of the same revision", async () => {
  const user = userEvent.setup();
  const f = tailnetFixture();
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: "Run test" }));
  expect(await screen.findByRole("list", { name: "4. Test the setup" }) !== null).toBe(true);
  expect(screen.queryByText("Test finished: 6 passed, 0 failed or unknown.") !== null).toBe(true);
  // An ordinary status observation of the same credential keeps the proof.
  act(() => f.send({ ...f.state.status, hostCount: 2, generation: f.state.status.generation }));
  await waitFor(() => expect(screen.queryByText(/2 hosts/) !== null).toBe(true));
  expect(checks() !== null).toBe(true);
  expect(view.container.textContent?.includes("Verified")).toBe(true);
  // Another tab replaces the credential: the hub keeps the generation and streams a new revision.
  act(() => f.send({ ...f.state.status, credentialRevision: "example-boot:2" }));
  await waitFor(() => expect(checks() === null).toBe(true));
  expect(view.container.textContent?.includes("Verified")).toBe(false);
});

it.each(["en-US", "de"] as const)("explains an ambiguous mint in %s: a test key may remain and expires, without promising a retry", async (locale) => {
  setAppLocale(locale);
  const user = userEvent.setup();
  const f = tailnetFixture();
  const ambiguous = allPass(f.state.status.credentialRevision!);
  ambiguous.checks[1] = { id: "agent-key", state: "fail", code: "tailnet.provider-unavailable" } as never;
  ambiguous.checks[2] = { id: "cleanup", state: "unknown" } as never;
  f.state.test = () => ok(ambiguous);
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  await user.click(await screen.findByRole("button", { name: locale === "de" ? "Test starten" : "Run test" }));
  await waitFor(() => expect(view.container.querySelector('[data-check="cleanup"]') !== null).toBe(true));
  const text = view.container.querySelector('[data-slot="tailnet-test-checks"]')!.textContent ?? "";
  expect(text.includes(locale === "de" ? "läuft nach fünf Minuten ab" : "expires after five minutes")).toBe(true);
  expect(/automatic|automatisch/i.test(text)).toBe(false);
  expect(view.container.textContent?.includes(locale === "de" ? "Geprüft" : "Verified")).toBe(false);
});

it("describes only the hub checks for manual keys", async () => {
  const f = tailnetFixture({ ...connectedStatus(), mode: "manual" });
  f.state.settings = { ...f.state.settings, mode: "manual" };
  renderWithApp(<TailnetConnection />, f.adapter);
  expect(await screen.findByText("Checks that the hub joined the tailnet with its tag and address. Manual keys create no test key.") !== null).toBe(true);
  expect(screen.queryByText(/Creates a tagged one-use key/) === null).toBe(true);
});
