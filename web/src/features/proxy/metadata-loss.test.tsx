// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original metadata loss consumer boundary and gesture proofs.


import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { Operations } from "./Operations";
import { RefusalView } from "./controls";
import { fixture, instance, operation, Providers } from "./proxy.test-support";

const loss = [
  { path: "routes/app.yml", attributes: ["system.posix_acl_access", "user.example"], links: 1, token: Array(65).join("c") },
  { path: "routes/other.yml", token: Array(65).join("d") },
];
const receipt = () => operation({ state: "rejected", phase: "staging", reason: "metadata-loss", metadataLoss: loss });
const writes = (f: ReturnType<typeof fixture>) => f.writes.filter(w => w.path.endsWith("/operations"));
const mount = (f: ReturnType<typeof fixture>) => render(<Providers><InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} /></Providers>);
async function open(kind: "edit" | "delete") {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: kind === "edit" ? "Edit route" : "Delete route" }));
  return user;
}
const warning = {
  "en-US": "This metadata loss is permanent. Restoring the old content does not bring the attributes back.",
  de: "Dieser Metadatenverlust ist dauerhaft. Das Wiederherstellen des alten Inhalts bringt die Attribute nicht zurück.",
};
const prospectiveWarning = {
  "en-US": "This metadata loss would be permanent. Restoring the old content would not bring the attributes back.",
  de: "Dieser Metadatenverlust wäre dauerhaft. Das Wiederherstellen des alten Inhalts würde die Attribute nicht zurückbringen.",
};
describe("metadata loss presentation", () => {
  it.each(["en-US", "de"] as const)("shows every detail and a prospective warning in %s", locale => {
    render(<Providers locale={locale}><RefusalView refusal={receipt()} /></Providers>);
    for (const text of [loss[0]!.path, "system.posix_acl_access", "user.example", loss[1]!.path, prospectiveWarning[locale],
      locale === "de" ? "1 weiterer Hardlink behält den alten Inhalt." : "1 other hard link keeps the old content."]) {
      expect(screen.queryByText(text) !== null).toBe(true);
    }
  });
  it("decodes and shows a resolver refusal in Recent operations", async () => {
    const f = fixture();
    const path = "config/static.yaml";
    const old = operation().old;
    const next = operation().new;
    f.state.operations = [operation({
      kind: "resolver", state: "refused", phase: "refused", reason: "unsupported-metadata",
      evidence: undefined, target: "", old: { present: false, hash: "" }, new: { present: false, hash: "" },
      resolver: {
        intent: {
          name: "Example", email: "admin@example.test", storage: "/certs/acme.json",
          ca: { kind: "custom", url: "https://ca.example.test/directory" },
          challenge: { kind: "http", entryPoint: "web", delayBeforeCheck: 0 },
          variables: [],
        },
        phase: "refused", recreateCommitted: false, files: [{ path, kind: "static", old, new: next }],
      },
      metadataLoss: [{ path, attributes: ["user.example"], token: loss[0]!.token }],
    })];
    const rows = await f.api.operations(instance.id);
    expect(rows[0]?.kind).toBe("resolver");
    render(<Providers><Operations rows={rows} busy={false} release={async () => {}} /></Providers>);
    const recent = within(screen.getByRole("region", { name: "Recent operations" }));
    expect(recent.queryByText(path) !== null).toBe(true);
    expect(recent.queryByText("user.example") !== null).toBe(true);
    expect(recent.queryByText(prospectiveWarning["en-US"]) !== null).toBe(true);
    expect(recent.queryByText(warning["en-US"]) === null).toBe(true);
    expect(recent.queryByRole("button", { name: "Retry the same change" }) === null).toBe(true);
  });
  it("shows restore losses read-only, including unknown attributes, without a retry", () => {
    render(<Providers><Operations rows={[operation({ state: "restored", metadataLoss: loss, diagnostic: { detail: "The original inode is gone; metadata cannot be recovered." } })]} busy={false} release={async () => {}} /></Providers>);
    expect(screen.queryByText(warning["en-US"]) !== null).toBe(true);
    expect(screen.queryByText("system.posix_acl_access") !== null).toBe(true);
    expect(screen.queryByText(loss[1]!.path) !== null).toBe(true);
    expect(screen.queryByText("The original inode is gone; metadata cannot be recovered.") !== null).toBe(true);
    expect(screen.queryByRole("button", { name: "Retry the same change" }) === null).toBe(true);
    expect(screen.queryByRole("checkbox") === null).toBe(true);
  });
});
describe.each(["edit", "delete"] as const)("confirmed %s retry", kind => {
  it("requires confirmation and retries the frozen request with exactly the reported loss", async () => {
    const f = fixture(); f.state.next = receipt(); mount(f);
    const user = await open(kind);
    await user.click(screen.getByRole("button", { name: kind === "edit" ? "Save" : "Delete route" }));
    const retry = await screen.findByRole("button", { name: "Retry the same change" }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    await user.click(retry);
    expect(writes(f)).toHaveLength(1);
    const original = writes(f)[0]!.body as { acknowledgements: object };
    expect("metadataLoss" in original.acknowledgements).toBe(false);
    expect(screen.queryByRole("dialog") !== null).toBe(true);
    // An edit after the receipt cannot silently change what its confirmation authorises.
    if (kind === "edit") fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "changed.example.test" } });
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("checkbox", { name: "I accept the permanent loss listed above for this change." }));
    expect(retry.disabled).toBe(false);
    f.state.next = operation({ state: "rejected", reason: "stale-hash" });
    await user.click(retry);
    await waitFor(() => expect(writes(f)).toHaveLength(2));
    expect(writes(f)[1]!.body).toEqual({ ...original, acknowledgements: { ...original.acknowledgements, metadataLoss: loss } });
    await waitFor(() => expect(screen.queryAllByText("A configuration file changed.").length).toBeGreaterThan(0));
    expect(screen.queryByRole("dialog") !== null).toBe(true);
  });
});
it("keeps the draft and shows the module update-required text after retry", async () => {
  const f = fixture(); f.state.next = receipt(); mount(f);
  const user = await open("edit");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("button", { name: "Retry the same change" });
  await user.click(within(screen.getByRole("dialog")).getByRole("checkbox", { name: "I accept the permanent loss listed above for this change." }));
  const raw = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    if (String(input).endsWith("/operations") && init?.method === "POST")
      return new Response(JSON.stringify({ success: false, code: "agent.update-required" }), { status: 409 });
    return raw(input, init);
  });
  await user.click(screen.getByRole("button", { name: "Retry the same change" }));
  await screen.findByText("The request could not be completed. Refresh to read the current state.");
  expect(screen.queryByText("The host must be updated before this action.") !== null).toBe(true);
  expect(screen.queryByRole("dialog") !== null).toBe(true);
  expect(screen.queryByText("Write outcome is not confirmed. Recent operations are being read; refresh them before starting another action.") === null).toBe(true);
});
it("sends no metadata acknowledgement without a prior receipt", async () => {
  const f = fixture(); mount(f);
  const user = await open("edit");
  expect(screen.queryByRole("button", { name: "Retry the same change" }) === null).toBe(true);
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes(f)).toHaveLength(1));
  expect((writes(f)[0]!.body as { acknowledgements: object }).acknowledgements).toEqual({ unresolvedReferences: [], replaceRunningConfig: false, restart: false });
});
