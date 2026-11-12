// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";
import ProxyPage from "./ProxyPage";
import { RouteEditor } from "./RouteEditor";
import { ProxyConnectionDialog } from "./ProxyConnectionDialog";
import { INITIAL_ROUTES, proxyStore } from "./model";
import { labStore } from "./lab-bridge";
const route = { ...INITIAL_ROUTES[0]!, id: "editor-draft", name: "editor-example", proxy: "caddy-example", resolver: "" };
const info = { proxies: [{ kind: "caddy" as const, container: "lab-example", image: "caddy:2.10.2", reachable: true, reload: "admin", capabilities: { dnsChallenge: false, labels: false, publicAcme: false, localCa: true }, hostnames: ["lab.example.com"] }], backends: ["http://192.0.2.80:8080"] };
beforeEach(() => { proxyStore.reset(); labStore.reset(); setAppLocale("en-US"); vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("no lab"))); });
afterEach(() => { cleanup(); proxyStore.reset(); labStore.reset(); setAppLocale("de"); vi.unstubAllGlobals(); });
describe("proxy editor and setup gestures", () => {
  it("round trips an incomplete Caddy draft, accepts renames, and retains invalid Advanced text", () => {
    renderWithApp(<RouteEditor initial={{ ...route, domain: "", url: "" }} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("radio", { name: "Advanced" }));
    expect((screen.getByRole("textbox", { name: "Caddyfile" }) as HTMLTextAreaElement).value.startsWith("# docklet-route:")).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "Easy" }));
    expect(screen.queryByRole("textbox", { name: "Caddyfile" }) === null).toBe(true);
  });
  it("uses one draft and preserves identity when Advanced changes the name", () => {
    renderWithApp(<RouteEditor initial={route} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("radio", { name: "Advanced" }));
    const text = screen.getByRole("textbox", { name: "Caddyfile" }) as HTMLTextAreaElement;
    fireEvent.change(text, { target: { value: text.value.replace("editor-example", "renamed") } });
    fireEvent.click(screen.getByRole("radio", { name: "Easy" }));
    expect((screen.getByRole("textbox", { name: "Route name" }) as HTMLInputElement).value).toBe("renamed");
    fireEvent.click(screen.getByRole("radio", { name: "Advanced" }));
    const editor = screen.getByRole("textbox", { name: "Caddyfile" }) as HTMLTextAreaElement;
    const invalid = editor.value.replace("encode gzip", "unknown_directive");
    fireEvent.change(editor, { target: { value: invalid } }); fireEvent.click(screen.getByRole("radio", { name: "Easy" }));
    expect(screen.getByRole("alert").textContent?.includes("single-site")).toBe(true);
    expect((screen.getByRole("textbox", { name: "Caddyfile" }) as HTMLTextAreaElement).value).toBe(invalid);
  });
  it("hides the live action without info and exposes a real adapt error without saving invalid text", async () => {
    labStore.set((s) => ({ ...s, info }));
    proxyStore.set((s) => ({ ...s, connections: { "caddy-example": { mode: "discovered", host: "example", container: "lab-example", image: "caddy:2.10.2", configPath: "/etc/caddy", apiUrl: "", lab: "caddy" } } }));
    vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({ ok: false, stage: "adapt", error: "snippet line 4: unknown directive" }), { status: 422 }));
    renderWithApp(<RouteEditor initial={route} onClose={() => {}} />);
    fireEvent.click(screen.getByRole("radio", { name: "Advanced" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Caddyfile" }), { target: { value: "invalid" } });
    fireEvent.click(screen.getByRole("button", { name: "Save & apply to lab" }));
    await screen.findByText("adapt · snippet line 4: unknown directive");
    expect(proxyStore.get().routes.some((r) => r.name === "editor-example")).toBe(false);
  });
  it("cleans up an unsaved Advanced apply before cancelling the draft", async () => {
    labStore.set((s) => ({ ...s, info }));
    proxyStore.set((s) => ({ ...s, connections: { "caddy-example": { mode: "discovered", host: "example", container: "lab-example", image: "caddy:2.10.2", configPath: "/etc/caddy", apiUrl: "", lab: "caddy" } } }));
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ ok: false, stage: "adapt", error: "snippet line 1: invalid" }), { status: 422 })).mockResolvedValueOnce(new Response(JSON.stringify({ ok: true })));
    const closed = vi.fn();
    renderWithApp(<RouteEditor creating initial={route} onClose={closed} />);
    fireEvent.click(screen.getByRole("radio", { name: "Advanced" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Caddyfile" }), { target: { value: "invalid" } });
    fireEvent.click(screen.getByRole("button", { name: "Save & apply to lab" }));
    await screen.findByText("adapt · snippet line 1: invalid");
    fireEvent.click(screen.getAllByRole("button", { name: "Close" })[0]!);
    await waitFor(() => expect(closed).toHaveBeenCalled());
    expect(vi.mocked(fetch).mock.lastCall?.[1]?.method).toBe("DELETE");
  });
  it("keeps unavailable lab controls hidden", async () => {
    renderWithApp(<RouteEditor initial={route} onClose={() => {}} />);
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Save & apply to lab" }) === null).toBe(true);
  });
  it("selects the lab instance and routes tab in the same navigation", async () => {
    labStore.set((s) => ({ ...s, info }));
    const { router } = renderWithApp(<ProxyPage />);
    fireEvent.click(await screen.findByRole("button", { name: "Choose proxy" }));
    fireEvent.click(await screen.findByRole("button", { name: /Lab · Caddy/ }));
    fireEvent.click(screen.getByRole("button", { name: "Set up instance" }));
    await screen.findByRole("button", { name: "Add route" });
    expect(new URLSearchParams(router.state.location.search).get("instance")).toBe(proxyStore.get().instance);
    expect(new URLSearchParams(router.state.location.search).get("tab")).toBe("routes");
  });
  it("retains the live connection until applied names can be deleted", async () => {
    labStore.set((s) => ({ ...s, info }));
    const connection = { mode: "discovered" as const, host: "example", container: "lab-example", image: "caddy:2.10.2", configPath: "/etc/caddy", apiUrl: "", lab: "caddy" as const };
    proxyStore.set((s) => ({ ...s, routes: [...s.routes, { ...route, labNames: [route.name] }], connections: { "caddy-example": connection } }));
    renderWithApp(<ProxyConnectionDialog instance="caddy-example" onClose={() => {}} onConfigured={() => {}} />);
    fireEvent.click(screen.getByRole("radio", { name: "Enter manually" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Container name / ID" }), { target: { value: "another-caddy" } });
    fireEvent.submit(document.getElementById("proxy-setup-form")!);
    expect(screen.getByRole("alert").textContent?.includes("applied lab routes")).toBe(true);
    expect(proxyStore.get().connections["caddy-example"]).toEqual(connection);
  });
  it("offers a manual kind choice and stores Caddy without borrowing a Traefik slot", async () => {
    const configured = vi.fn();
    renderWithApp(<ProxyConnectionDialog onClose={() => {}} onConfigured={configured} />);
    fireEvent.click(screen.getByRole("radio", { name: "Enter manually" }));
    fireEvent.keyDown(screen.getByRole("combobox", { name: "Proxy kind" }), { key: "ArrowDown" });
    fireEvent.click(await screen.findByRole("option", { name: "Caddy" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Container name / ID" }), { target: { value: "manual-caddy" } });
    fireEvent.submit(document.getElementById("proxy-setup-form")!);
    await waitFor(() => expect(configured).toHaveBeenCalled());
    const id = configured.mock.calls[0]![0] as string;
    expect(proxyStore.get().instances.find((p) => p.id === id)?.kind).toBe("caddy");
    expect(proxyStore.get().connections[id]?.configPath).toBe("/etc/caddy");
    expect(id === "atlas").toBe(false);
  });
});
