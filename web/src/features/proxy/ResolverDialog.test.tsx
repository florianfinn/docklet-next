// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

import { StrictMode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ResolverDialog } from "./ResolverDialog";
import { Settings } from "./Settings";
import { instance, inventory, operation, Providers } from "./proxy.test-support";

const reply = (data: unknown, status = 200) => new Response(JSON.stringify(data), {
  status, headers: { "Content-Type": "application/json", "X-CSRF-Token": "example" },
});
const config = { name: "NewResolver", email: "admin@example.test", storage: "/certs/acme.json", ca: { kind: "letsencrypt" },
  challenge: { kind: "http", entryPoint: "web", delayBeforeCheck: 0 }, variables: [] };
function receipt(state = "applied") {
  return { success: true, data: { ...operation(), kind: "resolver", state, phase: state,
    resolver: { intent: config, phase: state, files: [], recreateCommitted: true } } };
}
function fill() {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "NewResolver" } });
  fireEvent.change(screen.getByLabelText("ACME email"), { target: { value: "admin@example.test" } });
  fireEvent.click(screen.getByRole("checkbox", { name: /service will be recreated/ }));
}
afterEach(() => vi.unstubAllGlobals());
describe("resolver creation", () => {
  it("discloses administrator visibility and autoInjectEnv before one dispatch", async () => {
    const calls: {path: string; body?: object}[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input); calls.push({path, ...(init?.body ? { body: JSON.parse(String(init.body)) as object } : {})});
      return path.endsWith("/auth/mode") ? reply({}) : reply(receipt());
    }));
    render(<StrictMode><Providers><ResolverDialog instance={instance} inventory={inventory} close={() => {}} /></Providers></StrictMode>);
    expect(screen.getByText(/Administrators can read .env/).textContent).toContain("stack editor");
    expect(screen.getByText(/When autoInjectEnv/).textContent).toContain("other services");
    fill();
    fireEvent.click(screen.getByRole("button", { name: "Create certificate resolver" }));
    await waitFor(() => expect(screen.getByText("Applied").textContent).toBe("Applied"));
    const writes = calls.filter(call => call.path.endsWith("/resolvers"));
    expect(writes).toHaveLength(1);
    expect(writes[0]!.body).toEqual(config);
    expect(screen.queryByLabelText("Name")).toBeNull();
  });
  it("keeps an unknown outcome locked without retrying", async () => {
    let writes = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/auth/mode")) return reply({});
      writes++; throw new TypeError("network failure");
    }));
    render(<Providers><ResolverDialog instance={instance} inventory={inventory} close={() => {}} /></Providers>);
    fill();fireEvent.click(screen.getByRole("button", { name: "Create certificate resolver" }));
    await waitFor(() => expect(screen.getByText(/Write outcome is not confirmed/).textContent).toContain("refresh"));
    expect(screen.queryByRole("button", { name: "Create certificate resolver" })).toBeNull();
    expect(writes).toBe(1);
  });
  it("does not submit a byte-exact existing name", () => {
    render(<Providers><ResolverDialog instance={instance} inventory={inventory} close={() => {}} /></Providers>);
    fill();fireEvent.change(screen.getByLabelText("Name"), { target: { value: "existing-resolver" } });
    expect(screen.getByRole("button", { name: "Create certificate resolver" }).hasAttribute("disabled")).toBe(true);
  });
  it("limits creation to readable writable Traefik settings", () => {
    const view = render(<Providers><Settings instance={instance} inventory={inventory} busy={false} save={() => {}} remove={() => {}} /></Providers>);
    expect(screen.getByRole("button", { name: "Create certificate resolver" }).hasAttribute("disabled")).toBe(false);
    view.rerender(<Providers><Settings instance={instance} inventory={inventory} busy save={() => {}} remove={() => {}} /></Providers>);
    expect(screen.getByRole("button", { name: "Create certificate resolver" }).hasAttribute("disabled")).toBe(true);
    view.rerender(<Providers><Settings instance={{...instance,kind:"caddy"}} inventory={inventory} busy={false} save={() => {}} remove={() => {}} /></Providers>);
    expect(screen.queryByRole("button", { name: "Create certificate resolver" })).toBeNull();
  });
  it("clears DNS values before the live request completes and aborts them on unmount", async () => {
    const submitted = "dns-secret-sentinel";
    let complete: ((value: Response) => void) | undefined;
    let signal: AbortSignal | undefined;
    let sent: {variables: {name: string; value?: string}[]} | undefined;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith("/auth/mode")) return reply({});
      signal = init?.signal as AbortSignal;
      sent = JSON.parse(String(init?.body)) as typeof sent;
      return new Promise<Response>(resolve => { complete = resolve; });
    }));
    const view = render(<Providers><ResolverDialog instance={instance} inventory={inventory} close={() => {}} /></Providers>);
    fill();
    fireEvent.keyDown(screen.getByRole("combobox", { name: "Challenge" }), {key: "ArrowDown"});
    fireEvent.click(await screen.findByRole("option", {name: "DNS-01"}));
    fireEvent.keyDown(screen.getByRole("combobox", {name: "DNS provider"}), {key: "ArrowDown"});
    fireEvent.click(await screen.findByRole("option", {name: "HTTP request"}));
    fireEvent.change(screen.getByLabelText("HTTPREQ_ENDPOINT"), {target: {value: "http://sink.example.test"}});
    fireEvent.change(screen.getByLabelText("HTTPREQ_PASSWORD"), {target: {value: submitted}});
    fireEvent.click(screen.getByRole("checkbox", {name: "Reuse HTTPREQ_USERNAME from .env"}));
    fireEvent.click(screen.getByRole("button", {name: "Create certificate resolver"}));
    await waitFor(() => expect(sent?.variables).toEqual([
      {name: "HTTPREQ_ENDPOINT", value: "http://sink.example.test"}, {name: "HTTPREQ_USERNAME"}, {name: "HTTPREQ_PASSWORD", value: submitted},
    ]));
    expect((screen.getByLabelText("HTTPREQ_PASSWORD") as HTMLInputElement).value).toBe("");
    view.unmount();
    expect(signal?.aborted).toBe(true);
    complete?.(reply(receipt()));
  });

});
