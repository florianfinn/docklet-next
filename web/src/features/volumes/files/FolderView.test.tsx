// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original folder scale, later-page entry actions and identity reset guards.

import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import type { DirListing, FileEntry } from "@/domain/files";
import { renderWithApp } from "@/test/render";
import { FolderView } from "./FolderView";

const entries: FileEntry[] = Array.from({ length: 10_000 }, (_, i) => ({ name: `file-${i}.txt`, path: `/file-${i}.txt`, type: "file", size: i, modifiedAt: "2026-11-23T00:00:00Z", editable: true }));
const data: DirListing = { volumeId: "example~volume", path: "/", entries };
const rows = () => [...document.querySelectorAll("[data-folder-row]")];
const page = () => screen.getByRole("navigation", { name: "Pagination" }).textContent;
const callbacks = () => ({ onOpen: vi.fn(), onDownload: vi.fn(), onRename: vi.fn(), onMove: vi.fn(), onDelete: vi.fn() });
const listing = (next = data, isPlaceholderData = false) => ({ data: next, isPending: false, isError: false, isPlaceholderData, refetch: vi.fn() });
beforeEach(() => setAppLocale("en-US"));
afterEach(() => setAppLocale("de"));

it("bounds 10,000 entries to 200, pages forward and invokes later-page actions on the right entry", async () => {
  const actions = callbacks();
  renderWithApp(<FolderView listing={listing()} singleFile={false} {...actions} />);
  expect(rows().length).toBe(200);
  expect(page()).toContain("Page 1 of 50");
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(rows().length).toBe(200);
  expect(page()).toContain("Page 2 of 50");
  fireEvent.click(screen.getByRole("button", { name: "Open file-200.txt" }));
  expect(actions.onOpen.mock.calls[0]?.[0]).toEqual(entries[200]);
  const menu = screen.getByRole("button", { name: "Actions for file-200.txt" });
  for (const [name, callback] of [["Download", actions.onDownload], ["Rename", actions.onRename], ["Move", actions.onMove], ["Delete", actions.onDelete]] as const) {
    menu.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(await screen.findByRole("menuitem", { name }));
    expect(callback.mock.calls[0]?.[0]).toEqual(entries[200]);
  }
});

it("resets immediately for a requested path while old data is placeholder, then for volume and host", async () => {
  const actions = callbacks();
  const view = renderWithApp(<FolderView listing={listing(data, true)} singleFile={false} {...actions} />);
  fireEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(page()).toContain("Page 2 of 50");
  await act(() => view.router.navigate("/?path=%2Fnext"));
  expect(page()).toContain("Page 1 of 50");
  expect(rows().length).toBe(200);
  // Fresh listing identity also resets even when the route is unchanged.
  for (const volumeId of ["example~other-volume", "other-host~other-volume"]) {
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    view.router.routes[0]!.element = <FolderView listing={listing({ ...data, volumeId })} singleFile={false} {...actions} />;
    await act(() => view.router.revalidate());
    expect(page()).toContain("Page 1 of 50");
    expect(rows().length).toBe(200);
  }
});

it("clamps a later page after deletion and retains updated rename/upload entries", async () => {
  const actions = callbacks();
  const view = renderWithApp(<FolderView listing={listing({ ...data, entries: entries.slice(0, 401) })} singleFile={false} {...actions} />);
  const next = screen.getByRole("button", { name: "Next" });
  fireEvent.click(next); fireEvent.click(next);
  expect(rows().length).toBe(1);
  const renamed = { ...entries[200]!, name: "renamed.txt", path: "/renamed.txt" };
  view.router.routes[0]!.element = <FolderView listing={listing({ ...data, entries: [...entries.slice(0, 200), renamed] })} singleFile={false} {...actions} />;
  await act(() => view.router.revalidate());
  expect(page()).toContain("Page 2 of 2");
  expect(rows().length).toBe(1);
  fireEvent.click(screen.getByRole("button", { name: "Open renamed.txt" }));
  expect(actions.onOpen.mock.calls[0]?.[0]).toEqual(renamed);
  const uploaded = { ...entries[201]!, name: "uploaded.txt", path: "/uploaded.txt" };
  view.router.routes[0]!.element = <FolderView listing={listing({ ...data, entries: [...entries.slice(0, 200), renamed, uploaded] })} singleFile={false} {...actions} />;
  await act(() => view.router.revalidate());
  expect(page()).toContain("Page 2 of 2");
  expect(rows().length).toBe(2);
  fireEvent.click(screen.getByRole("button", { name: "Open uploaded.txt" }));
  expect(actions.onOpen.mock.calls[1]?.[0]).toEqual(uploaded);
});
