// Source: docklet-next/issues/549
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original adoption break-map editor assertions using existing production DTOs. AUTO-16 entry points under Keep current.

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { mapContainer } from "@/data/http/inventory-mapping";
import type { ContainerInfo } from "@/domain/model";
import { RouteEditor } from "./RouteEditor";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { fixture, instance, inventory, Providers, read, route } from "./proxy.test-support";
import type { Inventory, ReadResult, Route } from "./types";
import { ProxyError } from "@/data/http/proxy-api";

const data = vi.hoisted(() => ({ containers: [] as ContainerInfo[], narrow: false }));
vi.mock("@/data/queries", async importOriginal => ({
  ...await importOriginal<typeof import("@/data/queries")>(),
  useContainers: () => ({ data: data.containers, isPending: false }),
  useStacks: () => ({ data: [] }),
}));
vi.mock("@/shared/hooks/use-media-query", () => ({ useMediaQuery: () => data.narrow }));
function container(patch: Partial<ContainerInfo> = {}): ContainerInfo {
  return { ...mapContainer({ ...{ autoUpdateEnabled: false, command: "", created: 0, hostConfig: {}, imageId: "example-image", imagePlatform: "unknown", mounts: [], originSource: "detected" as const, ports: [], networkSettings: { networks: {} }, labels: {}, origin: "standalone" as const }, id: "backend-one", names: ["/example-backend-1"], image: "example/backend:1.0.0", state: "running", status: "Up", ports: [{ privatePort: 8080, publicPort: 18080, type: "tcp" }], networkSettings: { networks: { "example-network": {} } } }, "host-one", [], { stale: false, observedAt: "2026-11-21T00:00:00Z" }), ...patch };
}
beforeEach(() => { data.containers = []; data.narrow = false; });
function editor(options: { route?: Route; read?: ReadResult; inventory?: Inventory } = { route }) {
  const submit = vi.fn();
  const props = { instance, inventory, read, busy: false, submit, close: vi.fn(), ...options };
  const view = render(<Providers><RouteEditor {...props} /></Providers>);
  return { ...view, submit, props, user: userEvent.setup() };
}
function change(label: string, value: string) { fireEvent.change(screen.getByLabelText(label, { selector: "input,textarea" }), { target: { value } }); }
async function pick(user: ReturnType<typeof userEvent.setup>, label: string, name: string | RegExp) {
  screen.getByRole("combobox", { name: label }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name }));
}
async function destination(user: ReturnType<typeof userEvent.setup>) { await user.click(screen.getByRole("radio", { name: /Container \/ stack service/ })); }
const saved = (e: ReturnType<typeof editor>) => e.submit.mock.calls[0]![1];

it("row 1: a deliberate container pick sends only upstreams and preserves suffix and secondary upstreams", async () => {
  data.containers = [container()];
  const e = editor({ route: { ...route, upstreams: ["http://old.example.com:80/path?x=1", "http://secondary.example.com:90"] } });
  await destination(e.user);
  await pick(e.user, "Choose container", "example-backend-1");
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id, upstreams: ["http://example-backend-1:8080/path?x=1", "http://secondary.example.com:90"] });
});
it.each(["plain", "static", "mixed", "default", "unknown"])("row 2: new route without resolver states inherited TLS for %s entry points", async kind => {
  const inv = { ...inventory, entryPoints: kind === "unknown" ? [] : [{ name: "web", address: ":80", tls: false }, { name: "secure", address: ":443", tls: true }] };
  const e = editor({ inventory: inv });
  change("Name", "fresh"); change("Host / domain", "fresh.example.com"); change("Backend host", "backend.example.com");
  if (!["default", "unknown"].includes(kind)) await e.user.click(screen.getByRole("checkbox", { name: kind === "plain" ? /web ·/ : /secure ·/ }));
  if (kind === "mixed") await e.user.click(screen.getByRole("checkbox", { name: /web ·/ }));
  expect(screen.getByRole("radio", { name: /No route certificate/ }).getAttribute("data-state")).toBe("checked");
  if (["default", "unknown"].includes(kind)) expect(screen.queryByText(/default entry points apply and may serve TLS/) !== null).toBe(true);
  else if (kind !== "plain") expect(screen.queryByText(/entry point serves TLS even without/) !== null).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "create-route", name: "fresh", rule: "Host(`fresh.example.com`)", upstreams: ["http://backend.example.com"], ...(["default", "unknown"].includes(kind) ? {} : { entryPoints: kind === "plain" ? ["web"] : kind === "mixed" ? ["secure", "web"] : ["secure"] }) });
});
it("row 2: Keep current sends no resolver and leaves untouched entry points out", async () => {
  const e = editor({ route: { ...route, tls: true, resolver: "existing-resolver" } });
  await e.user.click(screen.getByRole("radio", { name: /Certificate from resolver/ }));
  await e.user.click(screen.getByRole("radio", { name: /Keep current/ }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id });
});
it("AUTO-16: entry points stay editable under Keep current and are sent alone", async () => {
  const e = editor({ route: { ...route, tls: true, resolver: "existing-resolver" } });
  const web = screen.getByRole("checkbox", { name: /web ·/ });
  expect(web.hasAttribute("disabled")).toBe(false);
  const before = route.entryPoints ?? [];
  await e.user.click(web);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  const expected = before.includes("web") ? before.filter(n => n !== "web") : [...before, "web"];
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id, entryPoints: expected });
});
it.each([false, true])("row 2: resolver choice sends an explicit resolver (existing=%s)", async existing => {
  const e = editor(existing ? { route } : {});
  if (!existing) { change("Name", "fresh"); change("Host / domain", "fresh.example.com"); change("Backend host", "backend.example.com"); }
  await e.user.click(screen.getByRole("radio", { name: /Certificate from resolver/ }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e).resolver).toBe("existing-resolver");
  expect("entryPoints" in saved(e)).toBe(false);
});
it("row 3: comma middleware chip is one exact reference", async () => {
  const e = editor({ route, read: { ...read, middlewares: [{ name: "a,b", kind: "local", file: route.file }] } });
  await e.user.click(screen.getByRole("button", { name: "a,b" }));
  expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e).middlewares).toEqual(["headers", "a,b"]);
});
it("row 4: only same-host running containers are offered", async () => {
  data.containers = [container(), container({ id: "host-two~backend", hostId: "host-two", name: "wrong-host" }), container({ id: "stopped", name: "stopped", state: "stopped" })];
  const e = editor(); await destination(e.user);
  screen.getByRole("combobox", { name: "Choose container" }).focus(); await e.user.keyboard("{ArrowDown}");
  await screen.findByRole("option", { name: "example-backend-1" });
  expect(screen.getAllByRole("option").map(o => o.textContent)).toEqual(["example-backend-1"]);
});
it.each(["udp", "many", "none"])("row 4: %s TCP port selection", async kind => {
  data.containers = [container({ ports: kind === "none" ? [] : kind === "udp" ? [{ container: "53", protocol: "udp" }, { container: "9000" }] : [{ container: "80", protocol: "tcp" }, { container: "443", protocol: "tcp" }, { container: "53", protocol: "udp" }] })];
  const e = editor(); await destination(e.user); await pick(e.user, "Choose container", "example-backend-1");
  expect((screen.getByLabelText("Backend port") as HTMLInputElement).value).toBe("");
  if (kind === "many") { await pick(e.user, "Container port", "443"); expect((screen.getByLabelText("Backend port") as HTMLInputElement).value).toBe("443"); }
  else expect(screen.queryByRole("combobox", { name: "Container port" }) === null).toBe(true);
});
it("row 4: refresh and disappearance never overwrite a picked draft", async () => {
  data.containers = [container()]; const e = editor(); await destination(e.user); await pick(e.user, "Choose container", "example-backend-1");
  change("Backend host", "operator.example.com"); change("Backend port", "9090");
  data.containers = [container({ name: "refreshed-backend", ports: [{ container: "7777", protocol: "tcp" }] })];
  e.rerender(<Providers><RouteEditor {...e.props} /></Providers>);
  expect((screen.getByLabelText("Backend host") as HTMLInputElement).value).toBe("operator.example.com");
  expect((screen.getByLabelText("Backend port") as HTMLInputElement).value).toBe("9090");
  data.containers = []; e.rerender(<Providers><RouteEditor {...e.props} /></Providers>);
  expect(screen.queryByText(/selected container is no longer available/) !== null).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e).upstreams).toEqual(["http://operator.example.com:9090"]);
});
it("row 5: an off-network container stays selectable with a warning; unknown networks make no claim", async () => {
  data.containers = [container({ id: "host-one~container-one", networks: ["proxy-network"] }), container({ networks: ["other-network"] })];
  const e = editor(); await destination(e.user);
  screen.getByRole("combobox", { name: "Choose container" }).focus(); await e.user.keyboard("{ArrowDown}");
  expect(screen.queryByRole("option", { name: /No shared network/ }) !== null).toBe(true);
  await e.user.click(screen.getByRole("option", { name: /No shared network/ }));
  expect(screen.queryByText("No shared network with the proxy; check reachability.") !== null).toBe(true);
  expect((screen.getByLabelText("Backend host") as HTMLInputElement).value).toBe("example-backend-1");
  data.containers = data.containers.map(c => ({ ...c, networks: [] })); e.rerender(<Providers><RouteEditor {...e.props} /></Providers>);
  expect(screen.queryByText("No shared network with the proxy; check reachability.") === null).toBe(true);
});
it.each(["shared", "opaque"])("keeps %s service destination suggestions unavailable", async kind => {
  const e = editor({ route: { ...route, ...(kind === "shared" ? { dependants: ["one", "two"] } : { upstreams: [] }) } });
  expect(screen.queryByRole("radio", { name: /Container \/ stack service/ }) === null).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id });
});
it("row 10: last-step Save validates hidden sections, announces and focuses the first invalid field", async () => {
  data.narrow = true; const e = editor({});
  for (let i = 0; i < 3; i++) await e.user.click(screen.getByRole("button", { name: "Next" }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(e.submit.mock.calls.length).toBe(0);
  expect(document.activeElement?.id).toBe("route-name");
  expect(screen.getByRole("alert").textContent).toBe("Complete the field: Name.");
  change("Name", "fresh"); change("Host / domain", "fresh.example.com");
  for (let i = 0; i < 3; i++) await e.user.click(screen.getByRole("button", { name: "Next" }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(e.submit.mock.calls.length).toBe(0); expect(document.activeElement?.id).toBe("backend-host");
  change("Backend host", "backend.example.com");
  await e.user.click(screen.getByRole("button", { name: "Next" })); await e.user.click(screen.getByRole("button", { name: "Next" }));
  await e.user.click(screen.getByRole("button", { name: "Save" })); expect(e.submit.mock.calls.length).toBe(1);
});
it("row 10: Enter advances a non-final step; draft and acknowledgements survive step and mode changes", async () => {
  data.narrow = true;
  const e = editor({ route, inventory: { ...inventory, runningDiverged: true, adminOff: true } });
  change("Name", "draft-name"); screen.getByLabelText("Name").focus(); await e.user.keyboard("{Enter}");
  expect(screen.queryByText(/Step 2 of 4/) !== null).toBe(true); expect(e.submit.mock.calls.length).toBe(0);
  change("Backend host", "draft.example.com");
  await e.user.click(screen.getByRole("radio", { name: /Advanced/ }));
  await e.user.click(screen.getByRole("radio", { name: /Easy/ }));
  expect((screen.getByLabelText("Backend host") as HTMLInputElement).value).toBe("draft.example.com");
  await e.user.click(screen.getByRole("button", { name: "Next" })); await e.user.click(screen.getByRole("button", { name: "Next" }));
  expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(true);
  await e.user.click(screen.getByRole("checkbox", { name: /replacing the running/ })); await e.user.click(screen.getByRole("checkbox", { name: /restarting this proxy/ }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id, name: "draft-name", upstreams: ["http://draft.example.com:8080"] });
  expect(e.submit.mock.calls[0]![2]).toEqual({ unresolvedReferences: [], replaceRunningConfig: true, restart: true });
});
it("row 10: a definite refusal retains the actual workspace draft", async () => {
  data.narrow = true; const f = fixture();
  f.api.operate = vi.fn(async () => { throw new ProxyError("invalid-input", undefined, 422); });
  render(<Providers><InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} /></Providers>);
  const user = userEvent.setup(); await user.click(await screen.findByRole("button", { name: "Actions for app" })); await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  change("Name", "retained"); for (let i = 0; i < 3; i++) await user.click(screen.getByRole("button", { name: "Next" }));
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(within(screen.getByRole("dialog")).queryByText("The request is invalid.") !== null).toBe(true));
  for (let i = 0; i < 3; i++) await user.click(screen.getByRole("button", { name: "Previous" }));
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("retained");
});
it("row 11: unchanged chip chain keeps order and omits middlewares on no-op", async () => {
  const r = { ...route, references: [{ name: "z", kind: "local" as const }, { name: "a", kind: "local" as const }] };
  const e = editor({ route: r, read: { ...read, routes: [r] } });
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id });
  expect((screen.getByLabelText("Middleware names (one per line)") as HTMLTextAreaElement).value).toBe("z\na");
});
it.each(["", "a\nb"])("row 11: freezes chain with unrepresentable name %j", async name => {
  const r = { ...route, references: [{ name, kind: "local" as const }] };
  const e = editor({ route: r, read: { ...read, middlewares: [{ name: "extra", kind: "local", file: route.file }] } });
  expect(screen.getByLabelText("Middleware names (one per line)").hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("button", { name: "extra" }).hasAttribute("disabled")).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" })); expect("middlewares" in saved(e)).toBe(false);
});
it("row 11: unresolved free entry still gates Save and preserves exact acknowledgement", async () => {
  const e = editor(); change("Middleware names (one per line)", "headers\nnew-reference");
  expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(true);
  await e.user.click(screen.getByRole("checkbox", { name: /I acknowledge unresolved/ }));
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e).middlewares).toEqual(["headers", "new-reference"]);
  expect(e.submit.mock.calls[0]![2].unresolvedReferences).toEqual(["new-reference"]);
});
it("row 12: domain-in-use warns but saves the exact rule and excludes the current route", async () => {
  const peer = { ...route, id: "peer", rule: "Host(`peer.example.com`)" };
  const e = editor({ route, read: { ...read, routes: [route, peer] } });
  expect(screen.queryByText(/already used by another route/) === null).toBe(true);
  change("Host / domain", "peer.example.com");
  expect(screen.queryByText(/peer.example.com is already used/) !== null).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(e.submit.mock.calls.length).toBe(1);
  expect(saved(e)).toEqual({ kind: "edit-route", routeId: route.id, rule: "Host(`peer.example.com`)" });
});
it("field help opens on keyboard focus and hover", async () => {
  const e = editor(); const help = screen.getByRole("button", { name: "Help for Backend host" });
  await act(async () => help.focus());
  expect((await screen.findByRole("tooltip")).textContent).toContain("Other upstreams and URL suffixes are preserved");
  await act(async () => help.blur()); await e.user.hover(help);
  expect((await screen.findByRole("tooltip")).textContent).toContain("Container name or external hostname");
});
it("AUTO-17: domains in use are a datalist on the domain field, never buttons", async () => {
  const other = { ...route, id: "other", name: "other", rule: "Host(`other.example.com`)" };
  const e = editor({ route, read: { ...read, routes: [route, other] } });
  const input = screen.getByLabelText("Host / domain", { selector: "input" });
  const list = document.getElementById(input.getAttribute("list") ?? "");
  expect([...(list?.querySelectorAll("option") ?? [])].map(o => o.getAttribute("value"))).toEqual(["other.example.com"]);
  expect(screen.queryByRole("button", { name: "other.example.com" }) === null).toBe(true);
  change("Host / domain", "other.example.com");
  expect(screen.queryByText(/already used by another route/) !== null).toBe(true);
  await e.user.click(screen.getByRole("button", { name: "Save" }));
  expect(saved(e).rule).toBe("Host(`other.example.com`)");
});
it.each([".", "./", "/etc/proxy", "/etc/proxy/./routes", "routes"])("a new route in route directory %s gets a canonical target", async dir => {
  const submit = vi.fn(), user = userEvent.setup();
  render(<Providers><RouteEditor instance={{ ...instance, routeDirectory: dir }} inventory={inventory} read={read} busy={false} submit={submit} close={vi.fn()} /></Providers>);
  change("Name", "fresh"); change("Host / domain", "fresh.example.com"); change("Backend host", "backend.example.com");
  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(submit.mock.calls[0]![0]).toBe(dir.includes("routes") ? "routes/fresh.yml" : "fresh.yml");
});
