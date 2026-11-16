// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager tests for the S5 R1 findings.

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { decodeProxyData } from "@/data/http/proxy-read-decoder";
import { InstanceWorkspace } from "./InstanceWorkspace";
import ProxyPage from "./ProxyPage";
import { ProxyError } from "@/data/http/proxy-api";
import { Diagnostic } from "./controls";
import { fixture, instance, operation, Providers, read, route } from "./proxy.test-support";
import type { Instance } from "./types";

const measured = "http://whoami-a:80";
const writes = (f: ReturnType<typeof fixture>) => f.writes.filter(w => w.path.endsWith("/operations"));
const change = (f: ReturnType<typeof fixture>) => (writes(f)[0]!.body as { change: Record<string, unknown> }).change;
function show(f: ReturnType<typeof fixture>, i: Instance = instance) {
  render(
    <Providers>
      <InstanceWorkspace instance={i} api={f.api} updated={() => {}} removed={() => {}} />
    </Providers>,
  );
}
async function edit() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  return user;
}

describe("proxy editor keeps server values the operator did not change", () => {
  it("never rewrites an untouched Traefik backend URL and keeps its written port", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, upstreams: [measured] };
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect("upstreams" in change(f)).toBe(false);
  });
  it("changes only the edited part of a backend URL", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, upstreams: [measured + "/base?x=1"] };
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "whoami-b" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).upstreams).toEqual(["http://whoami-b:80/base?x=1"]);
  });
  it("keeps a Caddy reverse_proxy target when only the name changes", async () => {
    const f = fixture(),
      i = { ...instance, kind: "caddy" as const };
    const text = `# docklet-route: app\napp.example.com {\n  route {\n    reverse_proxy ${measured}\n  }\n}\n`;
    f.state.read.routes = [
      { ...route, id: "routes/app.caddy", file: "routes/app.caddy", rule: "app.example.com", upstreams: [measured] },
    ];
    f.state.read.files = [{ ...f.state.read.files![0]!, path: "routes/app.caddy", content: text }];
    show(f, i);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).advanced).toBe(text.replace("# docklet-route: app", "# docklet-route: renamed"));
  });
  it("keeps a scheme-less Caddy upstream byte for byte", async () => {
    const f = fixture(),
      i = { ...instance, kind: "caddy" as const };
    const text = "# docklet-route: app\napp.example.com {\n  reverse_proxy whoami-a:80\n}\n";
    f.state.read.routes = [
      {
        ...route,
        id: "routes/app.caddy",
        file: "routes/app.caddy",
        rule: "app.example.com",
        upstreams: ["whoami-a:80"],
      },
    ];
    f.state.read.files = [{ ...f.state.read.files![0]!, path: "routes/app.caddy", content: text }];
    show(f, i);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Host / domain"), { target: { value: "next.example.com" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).advanced).toBe(text.replace("app.example.com {", "next.example.com {"));
  });
  it("omits empty defaults when creating a Traefik route", async () => {
    const f = fixture();
    show(f);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "New route" }));
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "fresh" } });
    fireEvent.change(screen.getByLabelText("Host / domain"), { target: { value: "fresh.example.com" } });
    fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "whoami-a" } });
    await user.click(screen.getByRole("checkbox", { name: /web/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    const sent = change(f);
    expect(["priority" in sent, "entryPoints" in sent, "middlewares" in sent]).toEqual([false, true, false]);
  });
});

afterEach(() => vi.unstubAllGlobals());
describe("proxy refusals and blocking states", () => {
  it("names the instance a discovered container is assigned to", async () => {
    const f = fixture();
    f.state.instances = [{ ...instance, id: "configured-instance", name: "Edge proxy" }];
    vi.stubGlobal("fetch", f.fetcher);
    render(
      <Providers>
        <ProxyPage />
      </Providers>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Add instance" }));
    screen.getByRole("combobox", { name: "Host" }).focus();
    await user.keyboard("{ArrowDown}");
    await user.click(await screen.findByRole("option", { name: "Example host" }));
    await screen.findAllByRole("article");
    expect(screen.queryByText("Assigned to Edge proxy") !== null).toBe(true);
  });
  it("says when TLS comes from the route's entry point", async () => {
    const f = fixture();
    f.state.inventory.entryPoints = [{ name: "websecure", address: ":443", tls: true }];
    f.state.read.routes![0] = { ...route, entryPoints: ["websecure"], tls: false };
    show(f);
    await edit();
    expect(screen.queryByText("Current TLS: provided by the entry point.") !== null).toBe(true);
  });
  it("keeps the editor open beside a rejected receipt's reason and diagnostic", async () => {
    const f = fixture();
    f.state.next = operation({
      state: "rejected",
      reason: "invalid-graph",
      diagnostic: { path: route.file, line: 4, detail: "unknown v3 matcher" },
    });
    show(f);
    const user = await edit();
    await user.click(screen.getByRole("button", { name: "Save" }));
    // Synchronise on the refresh that follows the receipt in every outcome, then assert.
    await waitFor(() => expect(f.state.operations.length).toBe(1));
    await act(async () => {});
    const dialog = screen.queryByRole("dialog");
    expect(dialog !== null).toBe(true);
    expect(within(dialog!).queryByText(/unknown v3 matcher/) !== null).toBe(true);
    expect(within(dialog!).queryByRole("button", { name: "Save" }) !== null).toBe(true);
  });
  it("blocks new writes while an operation is pending or needs recovery", async () => {
    for (const state of ["pending", "recovery-required"] as const) {
      const f = fixture();
      f.state.operations = [operation({ state, phase: "observing" })];
      f.state.next = operation({ state });
      const { unmount } = render(
        <Providers>
          <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
        </Providers>,
      );
      await screen.findByRole("heading", { name: "Routes" });
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "New route" }).hasAttribute("disabled")).toBe(true),
      );
      unmount();
    }
  });
  it("names the assigned instance and uses the Caddy delete hint", async () => {
    const f = fixture();
    f.state.read.routes = [{ ...route, upstreams: [measured] }];
    show(f, { ...instance, kind: "caddy" });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Actions for app" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete route" }));
    expect(screen.queryByText(/Its owned site block file is removed/) !== null).toBe(true);
    expect(screen.queryByText(/Traefik services/) === null).toBe(true);
  });
  it("rejects an unknown route class from the producer", () => {
    const body = { success: true, data: { ...read, routes: [{ ...route, class: "editable-ish" }] } };
    expect(() => decodeProxyData("read", body)).toThrow("proxy.invalid-response");
    expect(decodeProxyData("read", { success: true, data: read }).routes?.length).toBe(1);
  });
});

describe("verify round trip sweep (v-s5-fix1)", () => {
  it("keeps an untouched empty explicit port", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, upstreams: ["http://backend.example.com:/api?x=1"] };
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect("upstreams" in change(f)).toBe(false);
  });
  it("keeps a middleware name containing a comma on a rename", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, references: [{ name: "headers,strict", kind: "local", file: route.file }] };
    f.state.read.middlewares = [{ name: "headers,strict", kind: "local", file: route.file }];
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
    expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect("middlewares" in change(f)).toBe(false);
  });
  it("adds a selected definition whose name contains a comma as one reference", async () => {
    const f = fixture();
    f.state.read.middlewares = [{ name: "a,b", kind: "local", file: "routes/mw.yml" }];
    show(f);
    const user = await edit();
    screen.getByRole("combobox", { name: "Existing middleware" }).focus();
    await user.keyboard("{ArrowDown}");
    await screen.findByRole("option", { name: "headers" });
    await user.click(screen.getByRole("option", { name: "a,b" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).middlewares).toEqual(["headers", "a,b"]);
  });
  it("does not say TLS off for implicit entry points with TLS", async () => {
    const f = fixture();
    f.state.inventory.entryPoints = [{ name: "websecure", address: ":443", tls: true }];
    f.state.read.routes![0] = { ...route, entryPoints: [], tls: false };
    show(f);
    await edit();
    expect(screen.queryByText("Current TLS: off.") === null).toBe(true);
    expect(screen.queryByText("Current TLS: from the default entry points where they enable TLS.") !== null).toBe(true);
  });
  it.each([500, 503, undefined])("closes and locks on unknown status %s", async status => {
    const f = fixture();
    let calls = 0;
    f.api.operate = async () => {
      calls++;
      throw new ProxyError("invalid-graph", undefined, status);
    };
    show(f);
    const user = await edit();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls).toBe(1));
    await act(async () => {});
    expect(screen.queryByRole("dialog") === null).toBe(true);
    expect(screen.getByRole("button", { name: "New route" }).hasAttribute("disabled")).toBe(true);
    expect(calls).toBe(1);
  });
});

describe("verify 2 round trip sweep (v2-s5-fix)", () => {
  it("keeps an empty port separator when only the host changes", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, upstreams: ["http://backend.example.com:/api?x=1"] };
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "next.example.com" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).upstreams).toEqual(["http://next.example.com:/api?x=1"]);
  });
  it.each([" headers ", "headers\nstrict"])("keeps producer middleware name %j on rename", async name => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, references: [{ name, kind: "local", file: route.file }] };
    f.state.read.middlewares = [{ name, kind: "local", file: route.file }];
    show(f);
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
    expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect("middlewares" in change(f)).toBe(false);
  });
  it.each(["name", "domain"])("patches an indented Caddy %s leaf", async leaf => {
    const f = fixture(),
      i = { ...instance, kind: "caddy" as const };
    const text =
      "  # docklet-route: app\n  app.example.com {\n    route {\n      reverse_proxy http://whoami-a:80\n    }\n  }\n";
    f.state.read.routes = [
      {
        ...route,
        id: "routes/app.caddy",
        file: "routes/app.caddy",
        rule: "app.example.com",
        upstreams: ["http://whoami-a:80"],
      },
    ];
    f.state.read.files = [{ ...f.state.read.files![0]!, path: "routes/app.caddy", content: text }];
    show(f, i);
    const user = await edit();
    fireEvent.change(screen.getByLabelText(leaf === "name" ? "Name" : "Host / domain", { selector: "input" }), {
      target: { value: leaf === "name" ? "renamed" : "next.example.com" },
    });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect(change(f).advanced).toBe(
      leaf === "name"
        ? text.replace("docklet-route: app", "docklet-route: renamed")
        : text.replace("app.example.com {", "next.example.com {"),
    );
  });
});

it("reviewer keeps the untouched shared upstream line bytes", async () => {
  const f = fixture();
  const old = "http://backend.example.com/api ";
  f.state.read.routes = [{ ...route, upstreams: [old, "http://other.example.com:80"], dependants: ["app", "two"] }];
  show(f);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit shared service" }));
  fireEvent.change(screen.getByLabelText("Upstreams (one URL per line)"), {
    target: { value: old + "\nhttp://next.example.com:80" },
  });
  await user.click(screen.getByRole("checkbox", { name: "Confirm" }));
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(writes(f).length).toBe(1));
  expect(change(f).upstreams).toEqual([old, "http://next.example.com:80"]);
});

// Early smoke 4: overflow-wrap:break-word keeps a long token's min-content, so a Caddy
// JSON log line widened the dialog column to 654 px at 390 px. Only "anywhere" may apply.
it("wraps diagnostic text anywhere so long log tokens cannot widen the dialog", () => {
  render(
    <Providers>
      <Diagnostic value={{ path: "/etc/caddy/Caddyfile", detail: '{"file":"/etc/caddy/routes/example.caddy"}' }} />
    </Providers>,
  );
  const group = screen.getByRole("group", { name: "Diagnostic" });
  const pre = group.querySelector("pre")!;
  for (const el of [group, pre]) {
    expect(el.classList.contains("[overflow-wrap:anywhere]")).toBe(true);
    expect(el.classList.contains("break-words")).toBe(false);
  }
});
it("uses no break-word wrapping in proxy views", () => {
  const sources = import.meta.glob(["./*.tsx", "!./*.test.tsx"], { query: "?raw", import: "default", eager: true });
  const offenders = Object.entries(sources).filter(([, text]) => String(text).includes("break-words"));
  expect(offenders.map(([path]) => path)).toEqual([]);
});

// Candidate delta 1 F5: a recreated container makes read and inventory fail with
// container-changed; the registration must still be removable from its settings.
it("keeps Remove instance reachable when the instance cannot be read", async () => {
  const f = fixture();
  f.api.read = async () => {
    throw new ProxyError("container-changed", undefined, 409);
  };
  let removedCalls = 0;
  render(
    <Providers>
      <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => removedCalls++} />
    </Providers>,
  );
  const user = userEvent.setup();
  await screen.findByRole("alert");
  expect(screen.queryByRole("button", { name: "Remove instance" }) !== null).toBe(true);
  await user.click(screen.getByRole("button", { name: "Remove instance" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("checkbox", { name: "Confirm" }));
  await user.click(within(dialog).getByRole("button", { name: "Remove instance" }));
  await waitFor(() => expect(removedCalls).toBe(1));
  expect(f.writes.some(w => w.method === "DELETE" && w.path.endsWith("/instances/" + instance.id))).toBe(true);
});
