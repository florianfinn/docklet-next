// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original pre-check guidance and bounded command-dialog regressions (#231).

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { DockerImage } from "@/domain/images";
import { catalogs } from "@/app/i18n/catalog";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { renderWithApp } from "@/test/render";
import { HostUpdateSettings } from "@/features/settings/sections/LiveUpdatesSection";
import { PullDialog } from "@/features/images/ImageDialogs";
import { LiveRestorePoints, LiveStackUpdates } from "./live-stack-updates";
import { StackUpdateDialog } from "./stack-update-dialog";
import { DeletePointDialog } from "./delete-point-dialog";
const t = catalogs.de.common.stackUpdates;

it.each([false, true])("before any check missing baseline shows guidance and keeps restore usable (Edge=%s)", async (edge) => {
  const f = updateFixture(); f.state.isEdge = edge; await f.adapter.getCurrentUser();
  f.state.baseline.available = false; f.state.baseline.code = "update.baseline-unavailable"; f.state.policy.candidates = [];
  renderWithApp(<><LiveStackUpdates stackId="direct~project" /><LiveRestorePoints stackId="direct~project" /></>, f.adapter);
  await screen.findByRole("button", { name: t.editPolicy });
  await waitFor(() => expect(screen.queryByText(catalogs.de.images.errors["update_baseline-unavailable"]) !== null).toBe(true));
  expect(f.writes()).toHaveLength(0);
  const restore = await screen.findByRole("button", { name: t.restore });
  await waitFor(() => expect(restore.hasAttribute("disabled")).toBe(false)); await userEvent.click(restore);
  const dialog = within(screen.getByRole("dialog")); await userEvent.click(await dialog.findByRole("checkbox"));
  expect(dialog.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(false);
  await userEvent.click(dialog.getByRole("button", { name: t.restore })); await dialog.findByText(t.accepted);
  expect(f.writes()).toHaveLength(1); expect(f.writes()[0]!.path.endsWith("/restore")).toBe(true);
});
it.each(["update", "restore", "delete", "policy", "schedule", "pull"] as const)("%s dialog bounds long command values within the viewport", async (kind) => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  if (kind === "update" || kind === "restore") renderWithApp(<StackUpdateDialog stackId="direct~project" point={kind === "restore" ? point : undefined} onClose={() => {}} />, f.adapter);
  else if (kind === "delete") renderWithApp(<DeletePointDialog scope={{ kind: "stack", id: "direct~project" }} point={point} close={() => {}} refresh={async () => [point]} />, f.adapter);
  else if (kind === "policy") { renderWithApp(<LiveStackUpdates stackId="direct~project" />, f.adapter); await userEvent.click(await screen.findByRole("button", { name: t.editPolicy })); }
  else if (kind === "schedule") { renderWithApp(<HostUpdateSettings hostId="direct" name="Example" />, f.adapter); await userEvent.click(await screen.findByRole("button", { name: t.editSchedules })); }
  else renderWithApp(<PullDialog open onOpenChange={() => {}} hosts={[]} images={[]} initial={{ hostId: "direct", reference: `registry.example.com/example@sha256:${"a".repeat(64)}` }} />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  for (const name of ["min-w-0", "max-h-[calc(100dvh-2rem)]", "overflow-y-auto", "[overflow-wrap:anywhere]", "[&>*]:min-w-0"]) expect(dialog.classList.contains(name)).toBe(true);
  if (kind === "restore") {
    const consent = (await screen.findByRole("checkbox")).closest("label")!;
    expect(consent.classList.contains("min-w-0")).toBe(true);
    expect(consent.querySelector("span.break-all") !== null).toBe(true);
    const footer = within(dialog).getByRole("button", { name: t.restore }).parentElement!;
    expect(footer.classList.contains("flex-col-reverse")).toBe(true); expect(footer.classList.contains("sm:flex-row")).toBe(true);
  }
});


it("pull recognizes a canonical image when the operator enters a familiar reference", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const image: DockerImage = { id: "image", hostId: "direct", repository: "docker.io/library/busybox", tag: "1.35.0", aliases: [], digest: `sha256:${"a".repeat(64)}`, sizeMb: 1, createdAt: point.createdAt, architecture: "amd64", os: "linux", layers: 1, consumers: [] };
  renderWithApp(<PullDialog open onOpenChange={() => {}} hosts={[]} images={[image]} initial={{ hostId: "direct", reference: "busybox:1.35.0" }} />, f.adapter);
  expect(screen.queryByText(catalogs.de.images.pullDialog.known) !== null).toBe(true);
});
