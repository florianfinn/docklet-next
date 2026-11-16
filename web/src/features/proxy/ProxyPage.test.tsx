// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { readFileSync } from "node:fs";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { validateCatalog } from "@/app/i18n/catalog-validation.test-support";
import { enProxyContract } from "@/data/messages/proxy-en";
import { deProxyContract } from "@/data/messages/proxy-de";
import { InstanceWorkspace } from "./InstanceWorkspace";
import ProxyPage from "./ProxyPage";
import { fixture, instance, operation, Providers, route, hash, changedHash } from "./proxy.test-support";

const operationWrites = (f: ReturnType<typeof fixture>) => f.writes.filter(w => w.path.endsWith("/operations"));
async function workspace(f = fixture()) {
  render(<Providers><InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} /></Providers>);
  await screen.findByRole("heading", { name: "Routes" });
  return f;
}
async function openEdit(name = "app") {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: `Actions for ${name}` }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  return user;
}
async function choose(label: string, name: string) {
  const user = userEvent.setup();
  screen.getByRole("combobox", { name: label }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name }));
}
afterEach(() => vi.unstubAllGlobals());
describe("production proxy Manager HTTP integration", () => {
  it("creates setup with registered source and marks assigned containers", async () => {
    const f = fixture(); f.state.instances = []; vi.stubGlobal("fetch", f.fetcher);
    render(<Providers><ProxyPage /></Providers>);
    await screen.findByRole("heading", { name: "Set up proxy" });
    await choose("Host", "Example host");
    await screen.findByText("Assigned to configured-instance");
    const cards = screen.getAllByRole("article");
    expect(within(cards[1]!).getByRole("button", { name: "Register instance" }).hasAttribute("disabled")).toBe(true);
    fireEvent.change(within(cards[0]!).getByLabelText("Route directory"), { target: { value: "/etc/proxy/routes" } });
    await userEvent.click(within(cards[0]!).getByRole("button", { name: "Register instance" }));
    await waitFor(() => expect(f.writes.some(w => w.path.endsWith("/instances"))).toBe(true));
    expect(f.writes.find(w => w.path.endsWith("/instances"))?.body).toEqual({ containerId: instance.identity.containerId, environmentId: instance.environmentId, kind: "traefik", mountTarget: "/etc/proxy", name: "example-proxy", routeDirectory: "/etc/proxy/routes" });
    expect(f.fetcher.mock.calls.some(([url]) => String(url).includes("/discover?environmentId=host-one"))).toBe(true);
  });
  it("renders all route classes and links labels to the actual stack Compose editor", async () => {
    const f = fixture(); f.state.read.routes = [route, ...(["text-only", "infrastructure", "label-read-only"] as const).map((className, n) => ({ ...route, id: String(n), name: className, class: className, projectId: "project-one" }))];
    await workspace(f);
    expect(screen.getAllByRole("button", { name: /Actions for/ }).length).toBe(1);
    for (const name of ["text-only", "infrastructure", "label-read-only"]) expect(within(screen.getByRole("article", { name })).queryByRole("button") === null).toBe(true);
    expect(screen.getByRole("link", { name: "Open stack editor" }).getAttribute("href")).toBe("/stacks/host-one~project-one?tab=editor");
  });
  it("sends exact edited leaves and original expected/graph hashes", async () => {
    const f = await workspace(); const user = await openEdit();
    fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "next.example.com" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    expect(operationWrites(f)[0]?.body).toEqual({ target: "routes/app.yml", expected: { present: true, hash }, graphHashes: { "routes/app.yml": hash, "routes/extra.secret.yml": "opaque" }, change: { kind: "edit-route", routeId: route.id, name: "app", rule: route.rule, priority: 0, entryPoints: ["web"], middlewares: ["headers"], upstreams: ["http://next.example.com:8080"] }, acknowledgements: { replaceRunningConfig: false, restart: false, unresolvedReferences: [] } });
  });
  it.each(["unresolved", "diverged", "restart"])("gates %s acknowledgement before dispatch", async kind => {
    const f = fixture();
    f.state.inventory.runningDiverged = kind === "diverged"; f.state.inventory.adminOff = kind === "restart";
    await workspace(f); const user = await openEdit();
    if (kind === "unresolved") fireEvent.change(screen.getByLabelText("Middleware names (comma separated)"), { target: { value: "headers, hidden-middleware" } });
    const save = screen.getByRole("button", { name: "Save" });
    expect(save.hasAttribute("disabled")).toBe(true);
    fireEvent.submit(save.closest("form")!);
    expect(operationWrites(f).length).toBe(0);
    const label = kind === "unresolved" ? /I acknowledge unresolved/ : kind === "diverged" ? /I confirm replacing/ : /I confirm restarting/;
    await user.click(screen.getByRole("checkbox", { name: label }));
    await user.click(save);
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    const body = operationWrites(f)[0]!.body as { acknowledgements: object };
    expect(body.acknowledgements).toEqual({ unresolvedReferences: kind === "unresolved" ? ["hidden-middleware"] : [], replaceRunningConfig: kind === "diverged", restart: kind === "restart" });
  });
  it("keeps an opaque service router editable without sending upstreams", async () => {
    const f = fixture(); f.state.read.routes = [{ ...route, upstreams: [], serviceFile: "routes/extra.secret.yml" }];
    await workspace(f); const user = await openEdit();
    expect(screen.queryByLabelText("Backend host") === null).toBe(true);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    expect(Object.hasOwn((operationWrites(f)[0]!.body as { change: object }).change, "upstreams")).toBe(false);
  });
  it("lists every shared-service dependant before sending edit-service", async () => {
    const f = fixture(); f.state.read.routes = [{ ...route, dependants: ["routes/app.yml#app", "routes/app.yml#two", "routes/app.yml#three"] }];
    await workspace(f); const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Actions for app" })); await user.click(await screen.findByRole("menuitem", { name: "Edit shared service" }));
    for (const name of f.state.read.routes[0]!.dependants!) expect(within(screen.getByRole("dialog")).getByText(name).textContent).toBe(name);
    fireEvent.change(screen.getByLabelText("Upstreams (one URL per line)"), { target: { value: "http://next.example.com:8080" } });
    await user.click(screen.getByRole("checkbox", { name: "Confirm" })); await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    expect((operationWrites(f)[0]?.body as { change: object }).change).toEqual({ kind: "edit-service", routeId: route.id, service: "app", upstreams: ["http://next.example.com:8080"] });
  });
  it.each([true, false])("offers missing import only for writable root=%s", async writable => {
    const f = fixture(); const i = { ...instance, kind: "caddy" as const, rootFile: "/etc/proxy/Caddyfile" };
    f.state.inventory = { ...f.state.inventory, importPresent: false, importLine: "import /etc/proxy/routes/*.caddy", rootFile: i.rootFile, eligibility: { writable, textOnly: false, reason: "import-missing" } };
    f.state.read.files!.push({ ...f.state.read.files![0]!, path: "Caddyfile" });
    render(<Providers><InstanceWorkspace instance={i} api={f.api} updated={() => {}} removed={() => {}} /></Providers>);
    await screen.findByText("import /etc/proxy/routes/*.caddy");
    expect(screen.queryByRole("button", { name: "Insert import line" }) !== null).toBe(writable);
    if (!writable) { expect(screen.getByText(/Add this exact line/).textContent?.includes("stack editor")).toBe(true); return; }
    await userEvent.click(screen.getByRole("button", { name: "Insert import line" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Confirm" })); await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    expect((operationWrites(f)[0]?.body as { target: string }).target).toBe("Caddyfile");
  });
  it("never offers insertion for an existing import", async () => { await workspace(); expect(screen.queryByRole("button", { name: "Insert import line" }) === null).toBe(true); });
  it("offers restore only for an owned file with last-verified hash drift", async () => {
    const f = fixture(), file = f.state.read.files![0]!;
    f.state.read.files = [file, { ...file, path: "routes/drift.yml", hash: changedHash }, { ...file, path: "routes/foreign.yml", owned: false, hash: changedHash }, { ...file, path: "routes/unverified.yml", lastVerifiedHash: undefined }];
    await workspace(f);
    expect(screen.getAllByRole("button", { name: "Restore last verified version" }).length).toBe(1);
    await userEvent.click(screen.getByRole("button", { name: "Restore last verified version" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Confirm" })); await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(operationWrites(f).length).toBe(1));
    expect((operationWrites(f)[0]?.body as { change: object }).change).toEqual({ kind: "restore-owned" });
  });
  it("re-reads operations after a lost response and never repeats create", async () => {
    const f = await workspace(); f.state.lost = true;
    await userEvent.click(screen.getByRole("button", { name: "New route" }));
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "new-app" } }); fireEvent.change(screen.getByLabelText("Host / domain"), { target: { value: "new.example.com" } }); fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "backend.example.com" } });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByText(/The response was lost/);
    expect(operationWrites(f).length).toBe(1);
    expect(screen.queryByRole("dialog") === null).toBe(true);
    expect(screen.getByRole("button", { name: "New route" }).hasAttribute("disabled")).toBe(true);
    expect(f.fetcher.mock.calls.filter(([u, init]) => String(u).endsWith("/operations") && !init?.body).length).toBeGreaterThan(1);
  });
  it("follows pending receipts to their final state", async () => {
    const f = fixture(); f.state.operations = [operation({ state: "pending", phase: "reloading", evidence: undefined })];
    await workspace(f); await screen.findByText("Reloading");
    await screen.findByText("Verified", {}, { timeout: 5000 });
    expect(f.state.operationReads).toBe(1);
  });
  it("uses complete bilingual proxy keys with matching ICU arguments", () => { expect(validateCatalog(enProxyContract, deProxyContract)).toEqual([]); });
  it("registers only the production lazy page behind ModuleRoute", () => {
    const source = readFileSync("src/app/router.tsx", "utf8");
    expect(source.includes('lazy(() => import("@/features/proxy/ProxyPage"))')).toBe(true);
    expect(source.includes('<ModuleRoute id="proxy" page={ProxyPage} />')).toBe(true);
    expect(source.includes("design-lab/proxy/ProxyPage")).toBe(false);
  });
});
