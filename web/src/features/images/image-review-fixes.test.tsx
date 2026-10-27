// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original review regression coverage for issue #230.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { useState } from "react";
import { catalogs } from "@/app/i18n/catalog";
import { imageRegistryFixture, mutations } from "@/data/http/image-registry.test-support";
import { formatBytes, formatDateTime } from "@/domain/format";
import type { DockerImage } from "@/domain/images";
import type { Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { PullDialog, PruneDialog } from "./ImageDialogs";
import { ImageUpdate } from "./ImageUpdate";
import { RemoveImageDialog } from "./RemoveImageDialog";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
afterEach(() => vi.clearAllMocks());
const t = catalogs.de.images;
const host = { id: "host", name: "Example host", status: "online" } as Host;
const initial = { hostId: "host", reference: "example/app:1" };
const image = { id: "host~sha256:abc", hostId: "host", repository: "example/app", tag: "1", aliases: [], digest: "sha256:abc", consumers: [], sizeMb: 1 } as unknown as DockerImage;
function barrier() { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; }
async function start(label: string) { const button = await screen.findByRole("button", { name: label }); await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await userEvent.click(button); }

it("locks the complete pull form while the stream is pending", async () => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise;
  renderWithApp(<PullDialog open onOpenChange={vi.fn()} hosts={[host, { ...host, id: "other", name: "Second" }]} images={[]} initial={initial} />, f.adapter);
  await start(t.pullDialog.submit); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  for (const name of [t.pullDialog.host, t.pullDialog.registry]) expect((screen.getByRole("combobox", { name }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByLabelText(t.pullDialog.reference).closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
  expect((screen.getByRole("button", { name: "redis:7.4-alpine" }).closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
  await userEvent.type(screen.getByLabelText(t.pullDialog.reference), "changed");
  await userEvent.click(screen.getByRole("button", { name: "redis:7.4-alpine" }));
  expect((screen.getByLabelText(t.pullDialog.reference) as HTMLInputElement).value).toBe(initial.reference);
  await act(async () => done.release());
});

describe.each(["close", "remount"] as const)("pull after %s", (detach) => {
  it.each([true, false])("notifies its success=%s without closing a replacement dialog", async (ok) => {
    const f = imageRegistryFixture(), done = barrier(), close = vi.fn(); f.state.delay = () => done.promise;
    if (!ok) f.state.stream = '{"error":{"code":"image.pull-failed"}}\n';
    const view = renderWithApp(<PullDialog open onOpenChange={close} hosts={[host]} images={[]} initial={initial} />, f.adapter);
    await start(t.pullDialog.submit); await waitFor(() => expect(mutations(f)).toHaveLength(1));
    if (detach === "close") await userEvent.click(screen.getByRole("button", { name: t.pullDialog.cancel }));
    view.unmount();
    renderWithApp(<PullDialog open onOpenChange={close} hosts={[{ ...host, id: "other", name: "Second" }]} images={[]} />, f.adapter);
    close.mockClear();
    await act(async () => done.release());
    await waitFor(() => expect(ok ? toast.success : toast.error).toHaveBeenCalledOnce());
    expect(ok ? toast.error : toast.success).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled(); expect(screen.queryByRole("alert") === null).toBe(true);
    expect(vi.mocked(ok ? toast.success : toast.error).mock.calls[0]?.[0]).toContain(ok ? initial.reference : t.errors["image_pull-failed"]);
  });
});
it("locks prune mode while cleanup runs and retains image names after inventory refresh", async () => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise;
  const digest = `sha256:${"a".repeat(64)}`;
  f.state.response = { status: 200, body: { success: true, data: { imagesDeleted: ["sha256:abc", digest], skipped: [{ id: "sha256:abc", code: "image.in-use" }], failed: [], spaceReclaimed: 1048576, spaceReclaimedKnown: true } } };
  let refresh!: () => void;
  function Inventory() { const [images, setImages] = useState([image]); refresh = () => setImages([]); return <PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={images} />; }
  renderWithApp(<Inventory />, f.adapter);
  await start(t.pruneDialog.confirm); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  const mode = screen.getByRole("radio", { name: t.pruneDialog.modes.unused });
  expect((mode.closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
  await userEvent.click(mode); expect(mode.getAttribute("aria-checked")).toBe("false"); expect(mutations(f)).toHaveLength(1);
  await act(async () => refresh());
  await act(async () => done.release());
  await screen.findByText(t.pruneDialog.report);
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("example/app:1") !== null).toBe(true);
  expect(dialog.textContent).toContain("sha256:aaaaaaaaaaaa"); expect(dialog.textContent).not.toContain(digest);
  expect(dialog.textContent).not.toContain("host~"); expect(dialog.textContent).toContain("1 MB");
});
it.each(["success", "report", "unknown", "rejected"] as const)("reports detached prune %s after scope remount", async (outcome) => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise;
  if (outcome === "success") f.state.response = { status: 200, body: { success: true, data: { imagesDeleted: ["sha256:abc"], skipped: [], failed: [], spaceReclaimed: 0, spaceReclaimedKnown: false } } };
  if (outcome === "unknown") f.state.failTransport = true;
  if (outcome === "rejected") vi.spyOn(f.adapter, "pruneImages").mockImplementation(async () => { await done.promise; throw new Error("untrusted"); });
  const view = renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await start(t.pruneDialog.confirm);
  if (outcome !== "rejected") await waitFor(() => expect(mutations(f)).toHaveLength(1));
  view.unmount(); renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[{ ...host, id: "other", name: "Second" }]} images={[]} />, f.adapter);
  await act(async () => done.release());
  await waitFor(() => expect(outcome === "success" ? toast.success : toast.error).toHaveBeenCalledOnce());
  expect(outcome === "success" ? toast.error : toast.success).not.toHaveBeenCalled(); expect(screen.queryByRole("alert") === null).toBe(true);
  if (outcome === "report") { expect(vi.mocked(toast.error).mock.calls[0]?.[0]).toContain("Example host: Entfernt: 1 · Übersprungen: 2 · Fehlgeschlagen: 1"); }
  if (outcome === "unknown") { expect(vi.mocked(toast.error).mock.calls[0]?.[0]).toContain(t.errors["image_outcome-unknown"]); }
});
it("formats streamed byte counts while awaiting completion", async () => {
  const f = imageRegistryFixture(); let controller!: ReadableStreamDefaultController<Uint8Array>;
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => String(input).endsWith("/pull") ? new Response(new ReadableStream({ start(value) { controller = value; value.enqueue(new TextEncoder().encode('{"status":"Downloading","progressDetail":{"current":1048576,"total":2097152}}\n')); } })) : original(input, init));
  renderWithApp(<PullDialog open onOpenChange={vi.fn()} hosts={[host]} images={[]} initial={initial} />, f.adapter);
  await start(t.pullDialog.submit);
  await waitFor(() => expect(screen.getByRole("status").textContent).toContain(`${formatBytes(1048576)} / ${formatBytes(2097152)}`));
  await act(async () => { controller.enqueue(new TextEncoder().encode('{"done":true}\n')); controller.close(); });
});
it("formats the image observation timestamp", () => {
  const checkedAt = "2026-10-25T14:23:00Z";
  renderWithApp(<ImageUpdate image={{ ...image, updateObservation: { state: "current", checkedAt } }} detail />);
  expect(screen.getByText(`Geprüft: ${formatDateTime(checkedAt)}`) !== null).toBe(true); expect(document.body.textContent).not.toContain(checkedAt);
});
it.each([true, false])("notifies detached remove success=%s without calling a stale page callback", async (ok) => {
  const f = imageRegistryFixture(), done = barrier(), removed = vi.fn(); f.state.delay = () => done.promise;
  if (!ok) f.state.response = { status: 409, body: { error: { code: "image.in-use" } } };
  const view = renderWithApp(<RemoveImageDialog image={image} host={host.name} onClose={vi.fn()} onRemoved={removed} />, f.adapter);
  await screen.findByRole("alertdialog"); await userEvent.click(screen.getByRole("button", { name: t.removeDialog.confirm }));
  await waitFor(() => expect(mutations(f)).toHaveLength(1)); view.unmount();
  await act(async () => done.release());
  await waitFor(() => expect(ok ? toast.success : toast.error).toHaveBeenCalledOnce()); expect(removed).not.toHaveBeenCalled();
});

it.each([true, false])("notifies pull success=%s after the mounted host scope changes", async (ok) => {
  const f = imageRegistryFixture(), done = barrier(), close = vi.fn(); f.state.delay = () => done.promise;
  if (!ok) f.state.stream = '{"error":{"code":"image.pull-failed"}}\n';
  let changeScope!: () => void;
  function Scope() { const [hosts, setHosts] = useState([host]); changeScope = () => setHosts([{ ...host, id: "other", name: "Second" }]); return <PullDialog open onOpenChange={close} hosts={hosts} images={[]} initial={initial} />; }
  renderWithApp(<Scope />, f.adapter);
  await start(t.pullDialog.submit); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  await act(async () => changeScope()); await act(async () => done.release());
  expect(close).not.toHaveBeenCalled(); expect(screen.queryByRole("alert") === null).toBe(true);
  await waitFor(() => expect(ok ? toast.success : toast.error).toHaveBeenCalledOnce());
});

it("keeps the mobile pull grid, fields and suggestions shrinkable", () => {
  const f = imageRegistryFixture();
  renderWithApp(<PullDialog open onOpenChange={vi.fn()} hosts={[host]} images={[]} initial={initial} />, f.adapter);
  const dialog = screen.getByRole("dialog");
  expect(dialog.classList.contains("min-w-0")).toBe(true);
  expect(dialog.querySelector("form")?.classList.contains("min-w-0")).toBe(true);
  for (const control of within(dialog).getAllByRole("combobox")) expect(control.classList.contains("min-w-0")).toBe(true);
  const suggestion = screen.getByRole("button", { name: "ghcr.io/example/app:1.2.0" });
  for (const name of ["min-w-0", "max-w-full", "whitespace-normal", "break-all"]) expect(suggestion.classList.contains(name)).toBe(true);
});
