// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original Direct/Edge dialog rejection acceptance regressions.
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { catalogs } from "@/app/i18n/catalog";
import { networkContainer, networkFixture, recordedNetworkResponses } from "@/data/http/network-commands.test-support";
import { renderWithApp } from "@/test/render";
import { CreateNetworkDialog } from "./CreateNetworkDialog";
import { ConnectNetworkDialog } from "./ConnectNetworkDialog";

afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); localStorage.clear(); });
const barrier = () => { let release!: () => void; const promise = new Promise<void>((done) => { release = done; }); return { promise, release }; };
const text = (locale: "de" | "en-US") => catalogs[locale].networks;
async function setup(edge: boolean, locale: "de" | "en-US") {
  setAppLocale(locale);
  const f = networkFixture(edge);
  const hosts = await f.adapter.listHosts();
  const created = vi.fn();
  function Harness() {
    const [open, setOpen] = useState(true);
    return <CreateNetworkDialog open={open} onOpenChange={setOpen} hosts={hosts} networks={[]} defaultHostId="host" onCreated={created} />;
  }
  renderWithApp(<Harness />, f.adapter);
  const t = text(locale).createDialog;
  const dialog = await screen.findByRole("dialog");
  await userEvent.click(within(dialog).getByRole("button", { name: t.next }));
  await userEvent.type(within(dialog).getByLabelText(t.name), "retained_network");
  await userEvent.type(within(dialog).getByLabelText(t.subnet), "198.51.100.0/24");
  await userEvent.click(within(dialog).getByRole("button", { name: t.next }));
  return { ...f, created, dialog, t };
}
describe.each([false, true])("dialog rejections (Edge=%s)", (edge) => {
  it.each(["de", "en-US"] as const)("keeps the review and draft, shows inline overlap in %s without toast, clears on edit and retries", async (locale) => {
    const error = vi.spyOn(toast, "error");
    const f = await setup(edge, locale);
    f.state.response = { body: { ...recordedNetworkResponses["create-invalid"], code: "network.overlap" }, status: 409 };
    const submit = within(f.dialog).getByRole("button", { name: f.t.submit }) as HTMLButtonElement;
    await userEvent.click(submit);
    await waitFor(() => expect(submit.disabled).toBe(false));
    const alert = within(f.dialog).getByRole("alert");
    expect(alert.textContent).toBe(text(locale).errors.overlap);
    expect(document.activeElement === alert).toBe(true);
    expect(f.dialog.querySelector('[aria-current="step"]')?.textContent).toContain(f.t.steps.review);
    expect(within(f.dialog).getByText("retained_network") !== null).toBe(true);
    expect(within(f.dialog).getByText("198.51.100.0/24") !== null).toBe(true);
    expect(error).not.toHaveBeenCalled();
    expect(f.created).not.toHaveBeenCalled();
    await userEvent.click(within(f.dialog).getByRole("button", { name: f.t.back }));
    const name = within(f.dialog).getByLabelText(f.t.name) as HTMLInputElement;
    expect(name.value).toBe("retained_network");
    expect((within(f.dialog).getByLabelText(f.t.subnet) as HTMLInputElement).value).toBe("198.51.100.0/24");
    await userEvent.type(name, "_edited");
    expect(f.dialog.querySelector('[data-slot="dialog-error"]') === null).toBe(true);
    f.state.response = undefined;
    await userEvent.click(within(f.dialog).getByRole("button", { name: f.t.next }));
    await userEvent.click(within(f.dialog).getByRole("button", { name: f.t.submit }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    expect(f.created).toHaveBeenCalledWith(expect.objectContaining({ name: "retained_network_edited" }));
  });
  it("shows outcome-unknown inline and retains inventory refresh", async () => {
    const error = vi.spyOn(toast, "error");
    const f = await setup(edge, "de");
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    f.state.failTransport = true;
    try {
      await userEvent.click(within(f.dialog).getByRole("button", { name: f.t.submit }));
      expect((await within(f.dialog).findByRole("alert")).textContent).toBe(text("de").errors["outcome-unknown"]);
      expect(changed).toHaveBeenCalledWith(["topology", "containers"]);
      expect(error).not.toHaveBeenCalled();
    } finally { stop(); }
  });
  it("toasts a translated server rejection after closing a pending create", async () => {
    const error = vi.spyOn(toast, "error");
    const f = await setup(edge, "de");
    const entered = barrier(); const done = barrier();
    const fetcher = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init) => {
      if (init?.method === "POST") { entered.release(); await done.promise; }
      return fetcher(input, init);
    });
    f.state.response = { body: { ...recordedNetworkResponses["create-invalid"], code: "network.overlap" }, status: 409 };
    await userEvent.click(within(f.dialog).getByRole("button", { name: f.t.submit }));
    await entered.promise;
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog") === null).toBe(true);
    await act(async () => done.release());
    await waitFor(() => expect(error).toHaveBeenCalledWith(text("de").errors.overlap, undefined));
  });
  it("shows a server invalid-ip rejection inline in connect review", async () => {
    const error = vi.spyOn(toast, "error");
    const f = networkFixture(edge); f.state.containers.push(structuredClone(networkContainer));
    const hosts = await f.adapter.listHosts(); const topology = await f.adapter.getTopology(); const containers = await f.adapter.listContainers();
    f.state.response = { body: { success: false, code: "network.invalid-ip", error: "Invalid address" }, status: 400 };
    const close = vi.fn();
    renderWithApp(<ConnectNetworkDialog request={{ mode: "connect", containerId: "host~container1", networkId: "host~net1" }} onClose={close} host={hosts.find((h) => h.id === "host")!} topology={topology} containers={containers} />, f.adapter);
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect((within(dialog).getByRole("button", { name: "Prüfen" }) as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(within(dialog).getByRole("button", { name: "Prüfen" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Verbinden" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(text("de").errors["invalid-ip"]);
    expect(error).not.toHaveBeenCalled(); expect(close).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("button", { name: "Zurück" }) !== null).toBe(true);
  });
});
