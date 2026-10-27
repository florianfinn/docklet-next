// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original review lifecycle regression coverage for issue #230.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { catalogs } from "@/app/i18n/catalog";
import { imageRegistryFixture, mutations } from "@/data/http/image-registry.test-support";
import { formatDateTime } from "@/domain/format";
import { renderWithApp } from "@/test/render";
import RegistriesPage from "./RegistriesPage";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
afterEach(() => vi.clearAllMocks());
const t = catalogs.de.registries;
function barrier() { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; }
describe.each(["save", "test", "delete"] as const)("pending registry %s", (operation) => {
  it.each([true, false])("reports success=%s after unmount", async (ok) => {
    const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise;
    const view = renderWithApp(<RegistriesPage />, f.adapter);
    await userEvent.click(await screen.findByRole("button", { name: operation === "save" ? t.edit : operation === "test" ? t.test : t.remove }));
    const dialog = await screen.findByRole(operation === "delete" ? "alertdialog" : "dialog");
    if (!ok) f.state.response = operation === "test" ? { status: 502, body: { ok: false, checkedAt: "2026-10-25T00:00:00Z", code: "registry.authentication-failed" } } : { status: 400, body: { error: { code: "registry.invalid-config" } } };
    await userEvent.click(within(dialog).getByRole("button", { name: operation === "save" ? t.save : operation === "test" ? t.test : t.remove }));
    await waitFor(() => expect(mutations(f)).toHaveLength(1));
    if (operation === "save") expect((within(dialog).getByLabelText(t.descriptionLabel).closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
    view.unmount();
    await act(async () => done.release());
    await waitFor(() => expect(ok ? toast.success : toast.error).toHaveBeenCalledOnce());
    expect(ok ? toast.error : toast.success).not.toHaveBeenCalled();
    const text = vi.mocked(ok ? toast.success : toast.error).mock.calls[0]?.[0];
    expect(text).toContain(ok ? "registry.example.com" : catalogs.de.images.errors[operation === "test" ? "registry_authentication-failed" : "registry_invalid-config"]);
  });
});
it("formats the saved registry test timestamp", async () => {
  const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.test }));
  const dialog = await screen.findByRole("dialog"); await userEvent.click(within(dialog).getByRole("button", { name: t.test }));
  const checkedAt = "2026-10-25T00:00:00Z";
  expect(await within(dialog).findByText(`Geprüft: ${formatDateTime(checkedAt)}`) !== null).toBe(true);
  expect(dialog.textContent).not.toContain(checkedAt);
});
