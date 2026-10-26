// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored list projection → detail/reload regressions (#231).

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { catalogs } from "@/app/i18n/catalog";
import { useStack } from "@/data/queries";
import { StackHeader } from "@/features/stacks/detail/StackHeader";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";
import { candidateProject, candidateUpdateInfo } from "./candidate-update.test-support";

function DetailHeader() { const query = useStack("first~project"); return query.data ? <StackHeader stack={query.data} host={undefined} /> : <p>Loading detail</p>; }
it.each([false, true])("list and detail retain checked service state after a real check and reload (Edge=%s)", async (edge) => {
  setAppLocale("en-US");
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!; f.state.hostIds = ["first"]; f.state.isEdge = edge;
  let checked = false;
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname;
    if (path.endsWith("/auth/me")) return new Response(JSON.stringify({ success: true, data: { id: "operator", username: "operator", locale: "en-US", requiresPasswordChange: false } }));
    if (path.endsWith("/auth/mode")) return new Response(JSON.stringify({ success: true, data: { mode: "local" } }));
    if (path.endsWith("/check")) { checked = true; return new Response(JSON.stringify({ success: true, data: candidateUpdateInfo() })); }
    const response = await original(input);
    if (!path.endsWith("/capabilities") && !path.endsWith("/projects") && !path.endsWith("/projects/project") && !path.endsWith("/containers")) return response;
    const body = await response.json();
    if (path.endsWith("/capabilities")) body.data.push({ operationId: "checkUpdates", available: true, resourceRef: { kind: url.searchParams.get("kind"), id: url.searchParams.get("id") } });
    else if (path.endsWith("/projects")) body.data = [{ ...candidateProject, ...(checked ? { updateInfo: candidateUpdateInfo() } : {}) }];
    else if (path.endsWith("/projects/project")) body.data = { ...candidateProject, runtimeServices: undefined };
    else body.data = [];
    if (body.pagination && Array.isArray(body.data)) body.pagination.totalItems = body.data.length;
    return new Response(JSON.stringify(body), { headers: response.headers });
  });
  await f.adapter.getCurrentUser();
  const initial = await f.adapter.getStack("first~project");
  expect(initial.services).toHaveLength(1); expect(initial.services[0]!.update.state).toBe("unchecked");
  const view = renderWithApp(<DetailHeader />, f.adapter);
  await screen.findByRole("heading", { name: "Example" });
  await userEvent.click(screen.getByRole("button", { name: catalogs["en-US"].stacks.detail.redeployMenu }));
  const check = await screen.findByRole("menuitem", { name: catalogs["en-US"].stacks.detail.actions.check });
  await waitFor(() => expect(check.getAttribute("aria-disabled") === "true").toBe(false)); await userEvent.click(check);
  await screen.findByRole("button", { name: /Update status.*1 update/i });
  const listed = await f.adapter.listStacks(), detail = await f.adapter.getStack("first~project");
  expect(detail.services[0]!.update).toEqual(listed[0]!.services[0]!.update);
  expect(detail.compose).toContain("services:");
  view.unmount(); renderWithApp(<DetailHeader />, f.adapter);
  await screen.findByRole("button", { name: /Update status.*1 update/i });
  expect(f.fetcher.mock.calls.some(([input]) => String(input).includes("/projects/project/updates"))).toBe(false);
});
