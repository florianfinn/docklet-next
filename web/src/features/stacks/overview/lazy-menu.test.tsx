// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #556 tests: stack row menus read their descriptors only when opened.

import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { resourceFixture } from "@/data/http/resources.test-support";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import StacksPage from "./StacksPage";

const ROWS = 100, more = catalogs.de.common.actions.more, check = catalogs.de.stacks.menu.check, redeploy = catalogs.de.stacks.menu.redeploy;
beforeEach(() => { preferencesStore.set((p) => ({ ...p, stacksLayout: "table" })); });
afterEach(() => { preferencesStore.reset(); localStorage.clear(); });

function fixture() {
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!;
  let hold: Promise<void> | undefined;
  f.state.hostIds = ["first"];
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/auth/me")) return new Response(JSON.stringify({ success: true, data: { id: "operator", username: "operator", requiresPasswordChange: false, isGlobalAdmin: true, locale: "de" } }));
    if (url.pathname.endsWith("/projects")) {
      const body = await (await original(input)).json() as { data: { id: string; name: string }[] };
      const [row] = body.data;
      return new Response(JSON.stringify({ ...body, data: Array.from({ length: ROWS }, (_, i) => ({ ...row, id: `p${i}`, name: `stack-${String(i).padStart(3, "0")}` })), pagination: { totalItems: ROWS } }));
    }
    if (url.pathname === "/api/capabilities" && url.searchParams.get("kind") === "stack") {
      const id = url.searchParams.get("id")!, wait = hold; if (wait) await wait;
      const operations = ["runStackCommand", "redeployStack", "saveStackFiles", "installStackUpdates", "checkUpdates"];
      return new Response(JSON.stringify({ success: true, data: operations.map((operationId) => ({ operationId, available: true, resourceRef: { kind: "stack", id } })) }));
    }
    if (url.pathname.endsWith("/check")) return new Response(JSON.stringify({ success: true, data: { status: "up_to_date", hasUpdate: false } }));
    return original(input);
  });
  const stackReads = (id?: string) => f.fetcher.mock.calls.filter(([url]) => { const u = new URL(String(url), "https://hub.example.com"); return u.pathname === "/api/capabilities" && u.searchParams.get("kind") === "stack" && (!id || u.searchParams.get("id") === id); }).length;
  return { f, stackReads, setHold: (p?: Promise<void>) => { hold = p; } };
}
const button = (name: string) => screen.getByRole("button", { name: `${more}: ${name}` });

it("100 stack rows read no scoped descriptor until a menu opens, then one per opened stack", async () => {
  const { f, stackReads } = fixture();
  renderWithApp(<StacksPage />, f.adapter);
  await screen.findByRole("button", { name: `${more}: stack-099` });
  await act(async () => { await new Promise((r) => setTimeout(r, 50)); });
  expect(stackReads()).toBe(0);
  // Keyboard opening of the ellipsis menu.
  button("stack-000").focus();
  await userEvent.keyboard("{Enter}");
  await screen.findByRole("menu");
  await waitFor(() => expect(stackReads("first~p0")).toBe(1));
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("menu") === null).toBe(true));
  // Reopening the same stack reuses its descriptors; another row reads its own once.
  await userEvent.click(button("stack-000"));
  await screen.findByRole("menu");
  await userEvent.keyboard("{Escape}");
  await userEvent.click(button("stack-001"));
  await screen.findByRole("menu");
  await waitFor(() => expect(stackReads("first~p1")).toBe(1));
  expect(stackReads("first~p0")).toBe(1);
  expect(stackReads()).toBe(2);
});

it("an entry stays disabled until its descriptor arrives, keeps focus, then runs its command", async () => {
  const { f, setHold } = fixture();
  renderWithApp(<StacksPage />, f.adapter);
  await screen.findByRole("button", { name: `${more}: stack-000` });
  let release!: () => void;
  setHold(new Promise<void>((resolve) => { release = resolve; }));
  button("stack-000").focus();
  await userEvent.keyboard("{Enter}");
  const menu = await screen.findByRole("menu");
  // Redeploy depends on its descriptor; check is always offered to admins and the command decides.
  const entry = within(menu).getByRole("menuitem", { name: redeploy });
  expect(entry.getAttribute("aria-disabled")).toBe("true");
  const focused = within(menu).getByRole("menuitem", { name: check });
  focused.focus();
  setHold(undefined); await act(async () => { release(); });
  await waitFor(() => expect(within(menu).getByRole("menuitem", { name: redeploy }).hasAttribute("aria-disabled")).toBe(false));
  expect(menu.isConnected).toBe(true);
  expect(document.activeElement === focused).toBe(true);
  await userEvent.click(within(menu).getByRole("menuitem", { name: check }));
  await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).endsWith("/projects/p0/check"))).toBe(true));
});

it("the row's context menu shows the same entries and reads only that stack", async () => {
  const { f, stackReads } = fixture();
  renderWithApp(<StacksPage />, f.adapter);
  const row = (await screen.findByRole("button", { name: `${more}: stack-002` })).closest("tr")!;
  expect(stackReads()).toBe(0);
  fireEvent.contextMenu(row);
  const menu = await screen.findByRole("menu");
  expect(within(menu).queryByRole("menuitem", { name: check }) !== null).toBe(true);
  await waitFor(() => expect(stackReads("first~p2")).toBe(1));
  expect(stackReads()).toBe(1);
});
