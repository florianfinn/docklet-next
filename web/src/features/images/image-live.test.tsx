// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { catalogs } from "@/app/i18n/catalog";
import { imageRegistryFixture, mutations } from "@/data/http/image-registry.test-support";
import { renderWithApp } from "@/test/render";
import { PullDialog, PruneDialog } from "./ImageDialogs";
import { ImageUpdate } from "./ImageUpdate";
import { RemoveImageDialog } from "./RemoveImageDialog";
import type { DockerImage } from "@/domain/images";
import type { Host } from "@/domain/model";

const t = catalogs.de.images;
const host = { id: "host", name: "Example host", status: "online" } as Host;
const image: DockerImage = { id: "host~image", hostId: "host", repository: "example/app", tag: "1", aliases: [], digest: "", createdAt: "", sizeMb: 1, architecture: "", os: "", layers: 0, consumers: [] };
it("pulls via a saved registry host, with no inline credentials or registryId", async () => {
  const f = imageRegistryFixture(), close = vi.fn();
  renderWithApp(<PullDialog open onOpenChange={close} hosts={[host]} images={[]} />, f.adapter);
  const dialog = await screen.findByRole("dialog", { name: t.pullDialog.title });
  await waitFor(() => expect((within(dialog).getByRole("button", { name: t.pullDialog.submit }) as HTMLButtonElement).disabled).toBe(true));
  await waitFor(() => expect(f.state.calls.some((c) => c.path.endsWith("container-registries"))).toBe(true));
  within(dialog).getByRole("combobox", { name: t.pullDialog.registry }).focus(); await userEvent.keyboard("{ArrowDown}");
  await userEvent.click(await screen.findByRole("option", { name: "https://registry.example.com" }));
  await userEvent.type(within(dialog).getByLabelText(t.pullDialog.reference), "team/app:1");
  await userEvent.click(within(dialog).getByRole("button", { name: t.pullDialog.submit }));
  await waitFor(() => expect(close).toHaveBeenCalledWith(false));
  expect(mutations(f)[0]?.body).toEqual({ imageName: "registry.example.com/team/app:1" });
});
it("preserves a rejected pull draft and renders its typed error inline", async () => {
  const f = imageRegistryFixture(), close = vi.fn();
  f.state.response = { status: 409, body: { success: false, error: { code: "image.system-protected", message: "never expose" } } };
  renderWithApp(<PullDialog open onOpenChange={close} hosts={[host]} images={[]} />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  await userEvent.type(within(dialog).getByLabelText(t.pullDialog.reference), "example/app:1");
  await waitFor(() => expect((within(dialog).getByRole("button", { name: t.pullDialog.submit }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(within(dialog).getByRole("button", { name: t.pullDialog.submit }));
  expect((await within(dialog).findByRole("alert")).textContent).toBe(t.errors["image_system-protected"]);
  expect((within(dialog).getByLabelText(t.pullDialog.reference) as HTMLInputElement).value).toBe("example/app:1"); expect(close).not.toHaveBeenCalled();
});
it("shows actual prune outcomes and unknown reclaimed space, retaining the report", async () => {
  const f = imageRegistryFixture(), close = vi.fn();
  renderWithApp(<PruneDialog open onOpenChange={close} hosts={[host]} images={[image]} />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect((within(dialog).getByRole("button", { name: t.pruneDialog.confirm }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(within(dialog).getByRole("button", { name: t.pruneDialog.confirm }));
  await within(dialog).findByText(t.pruneDialog.report);
  expect(within(dialog).getByText("Entfernt: 1") !== null).toBe(true);
  expect(within(dialog).getByText("Übersprungen: 2") !== null).toBe(true);
  expect(within(dialog).getByText("Fehlgeschlagen: 1") !== null).toBe(true);
  expect(within(dialog).getByText(t.pruneDialog.spaceUnknown) !== null).toBe(true);
  expect(dialog.textContent).toContain(t.errors["image_in-use"]); expect(dialog.textContent).toContain(t.errors["image_system-protected"]);
  expect(close).not.toHaveBeenCalled();
});
it("sends force only as a request and retains authoritative in-use rejection", async () => {
  const f = imageRegistryFixture(), close = vi.fn();
  f.state.response = { status: 409, body: { success: false, error: { code: "image.in-use", message: "never expose" } } };
  renderWithApp(<RemoveImageDialog image={image} host="Example host" onClose={close} onRemoved={vi.fn()} />, f.adapter);
  const dialog = await screen.findByRole("alertdialog");
  await waitFor(() => expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("kind=image"))).toBe(true));
  await userEvent.click(within(dialog).getByLabelText(t.removeDialog.force));
  await userEvent.click(within(dialog).getByRole("button", { name: t.removeDialog.confirm }));
  expect((await within(dialog).findByRole("alert")).textContent).toBe(t.errors["image_in-use"]); expect(close).not.toHaveBeenCalled();
  const wire = f.fetcher.mock.calls.find(([, init]) => init?.method === "DELETE"); expect(String(wire?.[0])).toContain("force=true");
});
describe.each(["version", "rebuild", "unknown"] as const)("image update %s", (state) => {
  it("renders a read-only state without update actions or raw errors", () => {
    renderWithApp(<ImageUpdate image={{ ...image, updateObservation: { state, code: "update.check-unavailable" } }} detail />);
    expect(screen.getByText(t.observation[state]) !== null).toBe(true);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });
});
it("drops a late pull result after closing and reopening", async () => {
  const f = imageRegistryFixture(), close = vi.fn(); let release!: () => void;
  f.state.delay = () => new Promise<void>((resolve) => { release = resolve; });
  const view = renderWithApp(<PullDialog open onOpenChange={close} hosts={[host]} images={[]} initial={{ hostId: "host", reference: "example/app:1" }} />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect((within(dialog).getByRole("button", { name: t.pullDialog.submit }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(within(dialog).getByRole("button", { name: t.pullDialog.submit }));
  await waitFor(() => expect(typeof release).toBe("function"));
  view.unmount();
  await act(async () => release()); expect(close).not.toHaveBeenCalled();
});
it("covers every published image problem code in both catalogues", () => {
  const source = readFileSync("../types/image/safety.go", "utf8");
  const codes = source.match(/enum:"([^"]+)"/)![1]!.split(",");
  for (const code of codes) for (const locale of ["de", "en-US"] as const) {
    expect(catalogs[locale].images.errors).toHaveProperty(code.replaceAll(".", "_"));
  }
});
it("disables the pull entry when the hub aggregate is available but no displayed host permits pull", async () => {
  const { default: ImagesPage } = await import("./ImagesPage");
  const f = imageRegistryFixture(); f.state.denied.add("pullImage:host");
  renderWithApp(<ImagesPage />, f.adapter);
  await screen.findByRole("button", { name: "Details zu example/app:1.0 öffnen" });
  expect((screen.getByRole("button", { name: t.pull }) as HTMLButtonElement).disabled).toBe(true);
});
it("keeps truncated pulls open and links the optional real activity for inspection", async () => {
  const { activityStore } = await import("@/features/activity/activity-store");
  const f = imageRegistryFixture(), close = vi.fn();
  f.state.stream = '{"type":"activity","activityId":"pull-action"}\n';
  renderWithApp(<PullDialog open onOpenChange={close} hosts={[host]} images={[]} initial={{ hostId: "host", reference: "example/app:1" }} />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  await waitFor(() => expect((within(dialog).getByRole("button", { name: t.pullDialog.submit }) as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(within(dialog).getByRole("button", { name: t.pullDialog.submit }));
  expect((await within(dialog).findByRole("alert")).textContent).toContain(t.errors["image_outcome-unknown"]);
  await userEvent.click(within(dialog).getByRole("button", { name: t.followActivity }));
  expect(activityStore.get().focusRunId).toBe("pull-action"); expect(close).not.toHaveBeenCalled();
  activityStore.reset();
});
