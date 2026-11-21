// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #556 tests: lists never report empty while a host in scope has not answered.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { ComponentType } from "react";
import { catalogs } from "@/app/i18n/catalog";
import { copyInventoryRecovery, inventoryHostsPending, inventoryRecovery, inventoryStructuralSharing } from "@/data/inventory-recovery";
import ContainersPage from "@/features/containers/ContainersPage";
import ImagesPage from "@/features/images/ImagesPage";
import VolumesPage from "@/features/volumes/VolumesPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";

afterEach(() => { preferencesStore.reset(); localStorage.clear(); });

const pages: [string, ComponentType, string][] = [
  ["containers", ContainersPage, catalogs.de.containers.emptyAll.title],
  ["images", ImagesPage, catalogs.de.images.emptyAll.title],
  ["volumes", VolumesPage, catalogs.de.volumes.emptyAll.title],
];
type Answer = "rows" | "empty" | "failure";

/** The second host answers only when released; the first answers at once. */
function heldSecond() {
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!;
  let release!: (answer: Answer) => void;
  const answer = new Promise<Answer>((resolve) => { release = resolve; });
  f.fetcher.mockImplementation(async (input) => {
    if (!String(input).includes("/environments/second/")) return original(input);
    const how = await answer;
    if (how === "failure") return new Response(JSON.stringify({ success: false, error: { code: "docker.error", message: "raw diagnostic" } }), { status: 500, headers: { "content-type": "application/json" } });
    const response = await original(input);
    if (how === "rows" || !/\/(containers|images|volumes)$/.test(new URL(String(input), "https://hub.example.com").pathname)) return response;
    const body = await response.json() as { data: unknown };
    return new Response(JSON.stringify({ ...body, data: [], pagination: { totalItems: 0 } }), { status: response.status, headers: response.headers });
  });
  return { f, release };
}
// The first strip value is the page's total.
const total = () => document.querySelector("dl [role=group] dd > span")?.textContent;
const settle = () => new Promise((resolve) => setTimeout(resolve, 50));

for (const [name, Page, empty] of pages) {
  for (const answer of ["rows", "empty", "failure"] as const) {
    it(`${name}: with only the second host selected, no empty list or total until it answers (${answer})`, async () => {
      preferencesStore.set((p) => ({ ...p, hostSelection: ["second"] }));
      const { f, release } = heldSecond();
      renderWithApp(<Page />, f.adapter);
      // The first host's batch has been published; the second is still held.
      await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/environments/first/"))).toBe(true));
      await settle();
      expect(screen.queryByText(empty) === null).toBe(true);
      expect(total() ?? "–").toBe("–");
      release(answer);
      if (answer === "rows") await waitFor(() => expect(total()).not.toBe("–"));
      else await waitFor(() => expect(screen.queryByText(empty) !== null || (total() !== undefined && total() !== "–")).toBe(true));
    });
  }
  it(`${name}: with all hosts in scope, the first host's rows show while the total waits for the second`, async () => {
    const { f, release } = heldSecond();
    renderWithApp(<Page />, f.adapter);
    await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/environments/first/"))).toBe(true));
    await settle();
    expect(total() ?? "–").toBe("–");
    expect(screen.queryByText(empty) === null).toBe(true);
    release("rows");
    await waitFor(() => expect(total()).not.toBe("–"));
  });
}

it("changing the scope to a host that answered ends the wait without the held host", async () => {
  preferencesStore.set((p) => ({ ...p, hostSelection: ["second"] }));
  const { f } = heldSecond();
  renderWithApp(<ContainersPage />, f.adapter);
  await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/environments/first/"))).toBe(true));
  await settle();
  expect(total() ?? "–").toBe("–");
  act(() => preferencesStore.set((p) => ({ ...p, hostSelection: ["first"] })));
  await waitFor(() => expect(total()).toBe("2"));
});

it("answered hosts survive structural sharing of an equal result and the owner copy", () => {
  const recovery = inventoryRecovery();
  recovery.answered("first");
  const partial = recovery.partial<string[]>([]);
  expect(inventoryHostsPending(partial, ["first", "second"])).toBe(true);
  expect(inventoryHostsPending(partial, ["first"])).toBe(false);
  // An equal array is reused only while the answered hosts are the same; a new answer yields a new reference.
  const again = recovery.partial<string[]>([]);
  expect(inventoryStructuralSharing(partial, again) === partial).toBe(true);
  recovery.answered("second");
  const final = recovery.result<string[]>([]);
  const shared = inventoryStructuralSharing(partial, final) as object;
  expect(shared === partial).toBe(false);
  expect(inventoryHostsPending(shared, ["second"])).toBe(false);
  const copy: string[] = [];
  copyInventoryRecovery(partial, copy);
  expect(inventoryHostsPending(copy, ["second"])).toBe(true);
  // A result without metadata (cancelled or legacy) counts as settled.
  expect(inventoryHostsPending([], ["second"])).toBe(false);
});
