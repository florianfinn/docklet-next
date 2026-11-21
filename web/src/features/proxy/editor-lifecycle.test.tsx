// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import type { DockletAdapter } from "@/data/adapter";
import { InstanceWorkspace } from "./InstanceWorkspace";
import ProxyPage from "./ProxyPage";
import { fixture, instance, Providers, route, hash, changedHash } from "./proxy.test-support";

async function edit() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  return user;
}
const writes = (f: ReturnType<typeof fixture>) => f.writes.filter(w => w.path.endsWith("/operations"));
afterEach(() => vi.unstubAllGlobals());
describe("proxy editor snapshot and identity ownership", () => {
  it("keeps the original hash while the server file changes during an open draft", async () => {
    const f = fixture();
    render(
      <Providers>
        <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
      </Providers>,
    );
    const user = await edit();
    f.state.read.files![0]!.hash = changedHash;
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect((writes(f)[0]!.body as { expected: object }).expected).toEqual({ present: true, hash });
  });
  it("shows the route's current TLS and offers unused middleware definitions", async () => {
    const f = fixture();
    f.state.read.routes![0] = { ...route, tls: true, resolver: "existing-resolver" };
    f.state.read.middlewares = [{ name: "unused-auth", kind: "local", file: "routes/auth.yml" }];
    render(
      <Providers>
        <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
      </Providers>,
    );
    const user = await edit();
    expect(screen.queryByText("Current TLS: certificates from resolver existing-resolver.") !== null).toBe(true);
    expect(screen.queryByRole("button", { name: "unused-auth" }) !== null).toBe(true);
    await user.click(screen.getByRole("button", { name: "unused-auth" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    const body = writes(f)[0]!.body as {
      change: { middlewares?: string[] };
      acknowledgements: { unresolvedReferences: string[] };
    };
    expect(body.change.middlewares).toEqual(["headers", "unused-auth"]);
    expect(body.acknowledgements.unresolvedReferences).toEqual([]);
  });
  it("sends Advanced v3 rule and priority unchanged for server validation", async () => {
    const f = fixture();
    render(
      <Providers>
        <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
      </Providers>,
    );
    const user = await edit();
    await user.click(screen.getByRole("radio", { name: /Advanced/ }));
    fireEvent.change(screen.getByLabelText("Rule (Traefik v3)"), {
      target: { value: "Host(`app.example.com`) && PathPrefix(`/api`)" },
    });
    fireEvent.change(screen.getByLabelText("Priority"), { target: { value: "50" } });
    const resolver = screen.getByRole("combobox", { name: "TLS resolver" });
    resolver.focus();
    await user.keyboard("{ArrowDown}");
    await user.click(await screen.findByRole("option", { name: "existing-resolver" }));
    await user.click(screen.getByRole("radio", { name: /Easy/ }));
    expect((screen.getByLabelText("Host / domain") as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByText("Host(`app.example.com`) && PathPrefix(`/api`)").textContent).toBe(
      "Host(`app.example.com`) && PathPrefix(`/api`)",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect((writes(f)[0]!.body as { change: object }).change).toMatchObject({
      rule: "Host(`app.example.com`) && PathPrefix(`/api`)",
      priority: 50,
      resolver: "existing-resolver",
    });
  });
  it("retains Caddy directives in Easy and keeps modified Advanced text authoritative", async () => {
    const f = fixture(),
      i = { ...instance, kind: "caddy" as const };
    const text =
      "# docklet-route: app\napp.example.com {\n  route {\n    encode gzip\n    reverse_proxy http://backend.example.com:8080\n  }\n}\n";
    f.state.read.routes = [{ ...route, id: "routes/app.caddy", file: "routes/app.caddy", rule: "app.example.com" }];
    f.state.read.files = [{ ...f.state.read.files![0]!, path: "routes/app.caddy", content: text }];
    render(
      <Providers>
        <InstanceWorkspace instance={i} api={f.api} updated={() => {}} removed={() => {}} />
      </Providers>,
    );
    const user = await edit();
    fireEvent.change(screen.getByLabelText("Backend host"), { target: { value: "next.example.com" } });
    await user.click(screen.getByRole("radio", { name: /Advanced/ }));
    const advanced = screen.getByLabelText("Caddy site block") as HTMLTextAreaElement;
    expect(advanced.value).toBe(text.replace("backend.example.com", "next.example.com"));
    const custom = advanced.value.replace("encode gzip", "example_invalid_directive");
    fireEvent.change(advanced, { target: { value: custom } });
    await user.click(screen.getByRole("radio", { name: /Easy/ }));
    expect((screen.getByLabelText("Backend host").closest("fieldset") as HTMLFieldSetElement).disabled).toBe(true);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(writes(f).length).toBe(1));
    expect((writes(f)[0]!.body as { change: object }).change).toMatchObject({ advanced: custom });
  });
  it("ignores a held configuration response after session identity loss", async () => {
    const f = fixture(),
      raw = f.fetcher.getMockImplementation()!;
    let unblock!: () => void, started!: () => void;
    const held = new Promise<void>(resolve => {
        unblock = resolve;
      }),
      entered = new Promise<void>(resolve => {
        started = resolve;
      });
    f.fetcher.mockImplementation(async (input, init) => {
      if (String(input).endsWith("/read")) {
        started();
        await held;
      }
      return raw(input, init);
    });
    vi.stubGlobal("fetch", f.fetcher);
    let version = 0,
      active = true;
    const listeners = new Set<() => void>();
    const adapter = {
      kind: "http",
      subscribeChanges: vi.fn(() => () => {}),
      listHosts: async () => [{ id: "host-one", name: "Example host" }],
      getCurrentUser: vi.fn(),
    } as unknown as DockletAdapter;
    registerIdentityLifetime(adapter.subscribeChanges, {
      version: () => version,
      active: () => active,
      listen: fn => {
        listeners.add(fn);
        return () => {
          listeners.delete(fn);
        };
      },
    });
    render(
      <Providers adapter={adapter}>
        <ProxyPage />
      </Providers>,
    );
    await entered;
    await act(async () => {
      active = false;
      version++;
      for (const fn of [...listeners]) fn();
    });
    await act(async () => {
      unblock();
      await held;
    });
    expect(screen.queryByRole("heading", { name: "Proxy" }) === null).toBe(true);
    expect(screen.queryByRole("button", { name: "Actions for app" }) === null).toBe(true);
    expect(writes(f).length).toBe(0);
  });
});
