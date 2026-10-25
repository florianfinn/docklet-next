// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { imageRegistryFixture, mutations, registryDto } from "@/data/http/image-registry.test-support";
import { renderWithApp } from "@/test/render";
import RegistriesPage from "./RegistriesPage";

const t = catalogs.de.registries;
async function edit() { await userEvent.click(await screen.findByRole("button", { name: t.edit })); return screen.findByRole("dialog", { name: t.editTitle }); }
it("lists only the public projection and edits without prefilling or resubmitting credential identities", async () => {
  const f = imageRegistryFixture();
  f.state.registries = [{ ...registryDto, username: "hidden-identity", token: crypto.randomUUID(), awsAccessKeyId: crypto.randomUUID(), awsSecretAccessKey: crypto.randomUUID() }];
  renderWithApp(<RegistriesPage />, f.adapter);
  const dialog = await edit();
  expect(document.body.textContent).not.toMatch(/hidden-(identity|token|access|secret)/);
  expect((within(dialog).getByLabelText(t.username) as HTMLInputElement).value).toBe("");
  expect((within(dialog).getByLabelText(t.valueLabel) as HTMLInputElement).value).toBe("");
  await userEvent.clear(within(dialog).getByLabelText(t.descriptionLabel));
  await userEvent.type(within(dialog).getByLabelText(t.descriptionLabel), "Changed");
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  await waitFor(() => expect(mutations(f)).toHaveLength(1));
  expect(mutations(f)[0]?.body).not.toHaveProperty("token"); expect(mutations(f)[0]?.body).not.toHaveProperty("username");
});
it("keeps the draft after a typed save rejection and clears the stored credential explicitly", async () => {
  const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
  const dialog = await edit();
  await userEvent.clear(within(dialog).getByLabelText(t.descriptionLabel)); await userEvent.type(within(dialog).getByLabelText(t.descriptionLabel), "Keep this draft");
  f.state.response = { status: 409, body: { success: false, error: { code: "registry.exists", message: "never expose this" } } };
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  expect((await within(dialog).findByRole("alert")).textContent).toBe(catalogs.de.images.errors.registry_exists);
  expect((within(dialog).getByLabelText(t.descriptionLabel) as HTMLInputElement).value).toBe("Keep this draft");
  f.state.response = undefined;
  await userEvent.click(within(dialog).getByLabelText(t.clear));
  expect(within(dialog).queryByLabelText(t.valueLabel) === null).toBe(true);
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  await waitFor(() => expect(mutations(f)).toHaveLength(2));
  expect(mutations(f)[1]?.body).toMatchObject({ clearCredential: true, description: "Keep this draft" });
});
it("creates ECR configuration through keyboard Select gestures and write-only fields", async () => {
  const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.create }));
  const dialog = await screen.findByRole("dialog", { name: t.createTitle });
  await userEvent.type(within(dialog).getByLabelText(t.url), "https://registry.example.com");
  within(dialog).getByRole("combobox", { name: t.type }).focus(); await userEvent.keyboard("{ArrowDown}");
  await userEvent.click(await screen.findByRole("option", { name: t.ecr }));
  await userEvent.type(within(dialog).getByLabelText(t.region), "us-east-1");
  const access = crypto.randomUUID(), value = crypto.randomUUID();
  await userEvent.type(within(dialog).getByLabelText(t.keyIdentityLabel), access);
  await userEvent.type(within(dialog).getByLabelText(t.keyValueLabel), value);
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  await waitFor(() => expect(mutations(f)).toHaveLength(1));
  expect(mutations(f)[0]?.body).toMatchObject({ registryType: "ecr", awsRegion: "us-east-1", awsAccessKeyId: access, awsSecretAccessKey: value });
  expect(document.body.textContent).not.toContain(value);
});
describe.each(["registry.authentication-failed", "registry.unavailable", "registry.rate-limited"])("registry test %s", (code) => {
  it("shows the translated saved-test outcome inline without losing the session", async () => {
    const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
    await userEvent.click(await screen.findByRole("button", { name: t.test }));
    const dialog = await screen.findByRole("dialog", { name: t.testTitle });
    f.state.response = { status: code === "registry.rate-limited" ? 429 : 502, body: { ok: false, checkedAt: "2026-10-25T00:00:00Z", code } };
    await userEvent.click(within(dialog).getByRole("button", { name: t.test }));
    const text = (await within(dialog).findByRole("alert")).textContent;
    expect(text).toContain(catalogs.de.images.errors[code.replaceAll(".", "_") as "registry_unavailable"]);
    expect((await f.adapter.getCapability("listContainerRegistries")).available).toBe(true);
  });
});
it("keeps deletion open after server rejection", async () => {
  const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.remove }));
  const dialog = await screen.findByRole("alertdialog", { name: t.deleteTitle });
  f.state.response = { status: 500, body: { success: false, error: { code: "registry.persistence-failed", message: "never expose this" } } };
  await userEvent.click(within(dialog).getByRole("button", { name: t.remove }));
  expect((await within(dialog).findByRole("alert")).textContent).toBe(catalogs.de.images.errors["registry_persistence-failed"]);
});
it("disables target controls and unavailable reads using the live descriptor", async () => {
  const f = imageRegistryFixture(); f.state.denied.add("createContainerRegistry:0"); f.state.denied.add("updateContainerRegistry:saved");
  renderWithApp(<RegistriesPage />, f.adapter);
  await waitFor(() => expect((screen.getByRole("button", { name: t.create }) as HTMLButtonElement).disabled).toBe(true));
  expect((await screen.findByRole("button", { name: t.edit }) as HTMLButtonElement).disabled).toBe(true);
});
it("maps demo descriptors only for existing image and registry resources", async () => {
  const { createDemoAdapter } = await import("@/data/demo/demo-adapter");
  const adapter = createDemoAdapter();
  const registry = await adapter.createContainerRegistry({ url: "https://registry.example.com", description: "Example", registryType: "generic", enabled: true, insecure: false });
  for (const operation of ["updateContainerRegistry", "clearContainerRegistryCredential", "testContainerRegistry", "deleteContainerRegistry"]) {
    expect((await adapter.getCapability(operation, { kind: "registry", id: registry.id })).available).toBe(true);
    expect((await adapter.getCapability(operation, { kind: "registry", id: "missing" })).available).toBe(false);
  }
  const image = (await adapter.listImages())[0]!;
  expect((await adapter.getCapability("removeImage", { kind: "image", id: image.id })).available).toBe(true);
  expect((await adapter.getCapability("removeImage", { kind: "image", id: "missing" })).available).toBe(false);
});
it("accepts fresh generic credentials as masked inputs without echoing them after save", async () => {
  const f = imageRegistryFixture(); renderWithApp(<RegistriesPage />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: t.create }));
  const dialog = await screen.findByRole("dialog", { name: t.createTitle });
  const identity = crypto.randomUUID(), value = crypto.randomUUID();
  await userEvent.type(within(dialog).getByLabelText(t.url), "https://registry.example.com");
  await userEvent.type(within(dialog).getByLabelText(t.username), identity);
  await userEvent.type(within(dialog).getByLabelText(t.valueLabel), value);
  expect((within(dialog).getByLabelText(t.valueLabel) as HTMLInputElement).value).toBe(value);
  expect((within(dialog).getByLabelText(t.valueLabel) as HTMLInputElement).type).toBe("password");
  await userEvent.click(within(dialog).getByRole("button", { name: t.save }));
  await waitFor(() => expect(mutations(f)).toHaveLength(1));
  expect(mutations(f)[0]?.body).toMatchObject({ username: identity, token: value });
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  expect(document.body.textContent).not.toContain(value); expect(document.body.textContent).not.toContain(identity);
});
