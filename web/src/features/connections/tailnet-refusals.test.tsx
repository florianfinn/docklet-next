// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original strict transport and localized refusal rendering proofs.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createTailnet } from "@/data/http/tailnet";
import { renderWithApp } from "@/test/render";
import { TailnetConnection } from "./TailnetConnection";
import { connectedStatus, tailnetFixture } from "./tailnet.test-support";
import type { TestResult } from "@/domain/tailnet";

const ids = ["credential", "agent-key", "cleanup", "hub-joined", "hub-tag", "hub-address"] as const;
const result = (): TestResult => ({ credentialRevision: connectedStatus().credentialRevision, stale: false, checks: ids.map((id) => ({ id, state: "pass" })) });
const refusal = { code: "tailnet.tags-not-permitted" as const, refusedTags: ["tag:docklet-hub"] };
afterEach(() => setAppLocale("de"));

it.each(["test", "status", "problem"] as const)("keeps refusedTags through the %s decoder", async (path) => {
  const out = result(); out.checks[1] = { id: "agent-key", state: "fail", ...refusal };
  const state = { ...connectedStatus(), connection: { state: "error" as const, ...refusal }, provisioning: { state: "rejected" as const, ...refusal } };
  const controller = createTailnet("/api", { token: async () => "example", fetch: async () => path === "problem" ? Response.json({ ...refusal, status: 409 }, { status: 409 }) : Response.json({ success: true, data: path === "test" ? out : state }) }, () => 1);
  controller.observe(connectedStatus());
  if (path === "test") expect((await controller.api.test()).checks[1]?.refusedTags).toEqual(refusal.refusedTags);
  if (path === "status") {
    expect((await controller.api.status())?.connection.refusedTags).toEqual(refusal.refusedTags);
    expect(controller.api.current()?.provisioning.refusedTags).toEqual(refusal.refusedTags);
  }
  if (path === "problem") await expect(controller.api.test()).rejects.toMatchObject({ outcome: { code: refusal.code, status: 409 }, refusedTags: refusal.refusedTags });
});

it.each(["test", "status", "problem"] as const)("rejects unsafe or excessive refusedTags on %s", async (path) => {
  for (const refusedTags of [["not-a-tag"], ["tag:BAD"], Array(9).fill("tag:example"), "tag:example", [42]]) {
    const out = result(); Object.assign(out.checks[1]!, { ...refusal, refusedTags });
    const state = { ...connectedStatus(), connection: { state: "error", ...refusal, refusedTags } };
    const controller = createTailnet("/api", { token: async () => "example", fetch: async () => path === "problem" ? Response.json({ ...refusal, refusedTags, status: 409 }, { status: 409 }) : Response.json({ success: true, data: path === "test" ? out : state }) }, () => 1);
    controller.observe(connectedStatus());
    await expect(path === "status" ? controller.api.status() : controller.api.test()).rejects.toThrow("tailnet.invalid-response");
  }
});

it("keeps legacy absent fields valid without inventing refused tags", async () => {
  const out = result(); out.checks[1] = { id: "agent-key", state: "fail", code: refusal.code };
  const controller = createTailnet("/api", { token: async () => "example", fetch: async () => Response.json({ success: true, data: out }) }, () => 1);
  controller.observe(connectedStatus());
  expect((await controller.api.test()).checks[1]?.refusedTags).toBeUndefined();
  expect(controller.api.current()?.connection.refusedTags).toBeUndefined();
});

it.each(["en-US", "de"] as const)("renders mint and join refusals with their tags and recovery in %s", async (locale) => {
  setAppLocale(locale);
  const f = tailnetFixture({ ...connectedStatus(), connection: { state: "error", ...refusal }, routeAvailable: false });
  const out = result();
  out.checks[1] = { id: "agent-key", state: "fail", ...refusal, refusedTags: ["tag:docklet-agent"], providerStatus: 400 };
  out.checks[3] = { id: "hub-joined", state: "fail", ...refusal };
  f.state.test = () => Response.json({ success: true, data: out });
  const view = renderWithApp(<TailnetConnection />, f.adapter);
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain(locale === "de" ? "Abgelehnte Tags: tag:docklet-hub" : "Refused tags: tag:docklet-hub");
  expect(alert.textContent).toContain(locale === "de" ? "nur tag:docklet-hub" : "only tag:docklet-hub");
  await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Test starten" : "Run test" }));
  const list = await screen.findByRole("list", { name: locale === "de" ? "4. Einrichtung testen" : "4. Test the setup" });
  expect(list.textContent).toContain(locale === "de" ? "Abgelehnte Tags: tag:docklet-agent" : "Refused tags: tag:docklet-agent");
  expect(list.textContent).toContain(locale === "de" ? "Abgelehnte Tags: tag:docklet-hub" : "Refused tags: tag:docklet-hub");
  expect(list.textContent).toContain(locale === "de" ? "Speichere die Zugangsdaten erneut" : "Save the credential again");
  expect(view.container.textContent?.includes(locale === "de" ? "Schalte Tailscale" : "Turn on Tailscale")).toBe(false);
});

it.each(["en-US", "de"] as const)("renders the legacy join refusal without tags in %s", async (locale) => {
  setAppLocale(locale);
  const f = tailnetFixture(); const out = result();
  out.checks[3] = { id: "hub-joined", state: "fail", code: "tailnet.credential-rejected" };
  f.state.test = () => Response.json({ success: true, data: out });
  renderWithApp(<TailnetConnection />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: locale === "de" ? "Test starten" : "Run test" }));
  const list = await screen.findByRole("list");
  expect(within(list).queryByText(/Refused tags:|Abgelehnte Tags:/) === null).toBe(true);
  expect(list.textContent).toContain(locale === "de" ? "Speichere die Zugangsdaten erneut" : "Save the credential again");
});

it.each(["en-US", "de"] as const)("renders refused tags from an HTTP Problem in %s", async (locale) => {
  setAppLocale(locale);
  const f = tailnetFixture();
  f.state.test = () => Response.json({ ...refusal, status: 409 }, { status: 409 });
  renderWithApp(<TailnetConnection />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: locale === "de" ? "Test starten" : "Run test" }));
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain(locale === "de" ? "Abgelehnte Tags: tag:docklet-hub" : "Refused tags: tag:docklet-hub");
});

it("does not tell an enabled hub to turn on Tailscale while joining", async () => {
  setAppLocale("en-US");
  const f = tailnetFixture({ ...connectedStatus(), connection: { state: "joining" } });
  const out = result(); out.checks[3] = { id: "hub-joined", state: "fail", code: "tailnet.not-connected" };
  f.state.test = () => Response.json({ success: true, data: out });
  renderWithApp(<TailnetConnection />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Run test" }));
  const list = await screen.findByRole("list", { name: "4. Test the setup" });
  expect(list.textContent).toContain("Check the connection status");
  expect(list.textContent?.includes("Turn on")).toBe(false);
});
