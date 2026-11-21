// Source: docklet-next/docs/design/updates-ui.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #555 tests: a failed baseline read and a changed revision never reuse readiness or consent.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { expect, it } from "vitest";
import { renderWithApp } from "@/test/render";
import { catalogs } from "@/app/i18n/catalog";
import { StackUpdateDialog } from "./stack-update-dialog";
import { updateFixture, point, revision } from "@/data/http/stack-updates.test-support";
import { response } from "@/data/http/runtime-commands.test-support";

const t = catalogs.de.common.stackUpdates, failure = catalogs.de.images.errors["update_snapshot-unavailable"];
const other = "d".repeat(64), key = ["stack", "direct~project", "update-baseline"];

async function mount(restore: boolean, failing = false) {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  let fail = failing, refetch!: () => Promise<unknown>;
  f.state.hold = (path) => (fail && path.endsWith("/baseline") ? Promise.resolve(response({ success: false, error: { code: "update.snapshot-unavailable", message: "raw diagnostic" } }, 409)) : undefined);
  // The probe observes the same query; it renders in the same commit as the dialog.
  function Probe() {
    const client = useQueryClient(), probe = useQuery({ queryKey: key, enabled: false, queryFn: () => f.adapter.getUpdateBaseline!("direct~project") });
    refetch = () => client.refetchQueries({ queryKey: key });
    return <><output data-testid="probe">{probe.isError ? "error" : `${probe.data?.savedRevision}:${probe.dataUpdatedAt}`}</output><StackUpdateDialog stackId="direct~project" point={restore ? point : undefined} onClose={() => {}} /></>;
  }
  const view = renderWithApp(<Probe />, f.adapter);
  const dialog = within(await screen.findByRole("dialog")), send = () => dialog.getByRole("button", { name: restore ? t.restore : t.update });
  // Each refresh awaits the real query and the rendered result of that fetch before the caller asserts.
  const refresh = async (change: () => void) => {
    const before = screen.getByTestId("probe").textContent; change();
    await act(async () => { await refetch(); });
    await waitFor(() => expect(screen.getByTestId("probe").textContent).not.toBe(before));
  };
  return { f, dialog, send, refresh, unmount: view.unmount, setFail: (v: boolean) => { fail = v; } };
}
const checked = (box: HTMLElement) => box.getAttribute("aria-checked") === "true";
const posts = (f: ReturnType<typeof updateFixture>) => f.writes().filter((c) => c.path.endsWith("/update") || c.path.endsWith("/restore"));

it("an initially failed baseline read sends nothing, for update and restore", async () => {
  for (const restore of [false, true]) {
    const { f, dialog, send, unmount } = await mount(restore, true);
    await dialog.findByText(failure);
    expect(dialog.queryByRole("checkbox") === null).toBe(true);
    expect(dialog.queryByRole("button", { name: restore ? t.restore : t.update })?.hasAttribute("disabled") ?? true).toBe(true);
    if (dialog.queryByRole("button", { name: restore ? t.restore : t.update })) await userEvent.click(send());
    expect(posts(f)).toHaveLength(0);
    unmount();
  }
});

it("a failed refresh after a usable baseline disables update and drops restore consent", async () => {
  for (const restore of [false, true]) {
    const { f, dialog, send, refresh, setFail, unmount } = await mount(restore);
    if (restore) await userEvent.click(await dialog.findByRole("checkbox"));
    await waitFor(() => expect(send().hasAttribute("disabled")).toBe(false));
    await refresh(() => setFail(true));
    await dialog.findByText(failure);
    expect(send().hasAttribute("disabled")).toBe(true);
    await userEvent.click(send());
    await refresh(() => setFail(false));
    await waitFor(() => expect(dialog.queryByText(failure) === null).toBe(true));
    if (restore) { expect(checked(dialog.getByRole("checkbox"))).toBe(false); expect(send().hasAttribute("disabled")).toBe(true); }
    expect(posts(f)).toHaveLength(0);
    unmount();
  }
});

it("restore consent resets when the revision changes, also back to an earlier one", async () => {
  const { f, dialog, send, refresh } = await mount(true);
  await userEvent.click(await dialog.findByRole("checkbox"));
  await refresh(() => { f.state.baseline.savedRevision = other; });
  expect(checked(dialog.getByRole("checkbox"))).toBe(false);
  await refresh(() => { f.state.baseline.savedRevision = revision; });
  expect(checked(dialog.getByRole("checkbox"))).toBe(false);
  expect(send().hasAttribute("disabled")).toBe(true);
  await userEvent.click(dialog.getByRole("checkbox"));
  await userEvent.click(send());
  await waitFor(() => expect(posts(f)).toHaveLength(1));
  expect(JSON.parse(String(posts(f)[0]!.init.body))).toMatchObject({ expectedRevision: revision, replaceConfiguration: true });
});

it("restore consent resets after a revisionless status in between", async () => {
  const { f, dialog, send, refresh } = await mount(true);
  await userEvent.click(await dialog.findByRole("checkbox"));
  await refresh(() => Object.assign(f.state.baseline, { savedRevision: "", observedRevision: "", available: false, code: "update.config-drift" }));
  await dialog.findByText(t.restoreNeedsRevision);
  await refresh(() => Object.assign(f.state.baseline, { savedRevision: revision, observedRevision: revision, available: true, code: undefined }));
  expect(checked(await dialog.findByRole("checkbox"))).toBe(false);
  expect(send().hasAttribute("disabled")).toBe(true);
  expect(posts(f)).toHaveLength(0);
});
