// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 coverage of eligibility-driven setup (U3, break-map row U-3).

import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import ProxyPage from "./ProxyPage";
import { fixture, instance, Providers } from "./proxy.test-support";

afterEach(() => vi.unstubAllGlobals());

const refused = { kind: "volume", canonical: "example-volume", mountTarget: "/data", readOnly: true, mountMode: "rw", eligibility: { selectable: false, reason: "unsupported-metadata" } };
const config = { kind: "project", canonical: "/example/projects/proxy", projectPath: "proxy", mountTarget: "/etc/traefik", readOnly: false, mountMode: "ro", eligibility: { selectable: true, routeDirectory: { value: "dynamic", fixed: true } } };
const candidate = (sources: unknown[]) => [{ identity: instance.identity, kind: "traefik", image: "traefik:v3.7.16", sources }];

async function setup(sources: unknown[]) {
  const f = fixture();
  f.state.instances = [];
  f.state.discovery = candidate(sources);
  vi.stubGlobal("fetch", f.fetcher);
  render(
    <Providers>
      <ProxyPage />
    </Providers>,
  );
  await screen.findByRole("heading", { name: "Set up proxy" });
  const user = userEvent.setup();
  screen.getByRole("combobox", { name: "Host" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: "Example host" }));
  return { f, card: await screen.findByRole("article") };
}

it("U-3 defaults to the first selectable source, fixes its route directory and leads with the project path", async () => {
  const { f, card } = await setup([refused, config]);
  const select = within(card).getByRole("combobox", { name: "Configuration source" });
  expect(select.textContent?.startsWith("proxy → /etc/traefik")).toBe(true);
  const directory = within(card).getByLabelText("Route directory") as HTMLInputElement;
  expect([directory.value, directory.disabled]).toEqual(["dynamic", true]);
  expect(within(card).getByText("Fixed to the file provider directory of this Traefik container.") !== null).toBe(true);
  select.focus();
  await userEvent.keyboard("{ArrowDown}");
  const blocked = await screen.findByRole("option", { name: /example-volume → \/data/ });
  expect(blocked.getAttribute("data-disabled") !== null).toBe(true);
  await userEvent.keyboard("{Escape}");
  await userEvent.click(within(card).getByRole("button", { name: "Register instance" }));
  await waitFor(() => expect(f.writes.some(w => w.path.endsWith("/instances"))).toBe(true));
  expect(f.writes.find(w => w.path.endsWith("/instances"))?.body).toMatchObject({ mountTarget: "/etc/traefik", routeDirectory: "dynamic" });
});

it("U-3 without a selectable source explains the fix and offers no Create", async () => {
  const { card } = await setup([refused]);
  expect(within(card).getByRole("status").textContent?.includes("Mount the directory of the file provider")).toBe(true);
  expect(within(card).queryByRole("combobox", { name: "Configuration source" }) === null).toBe(true);
  expect((within(card).getByRole("button", { name: "Register instance" }) as HTMLButtonElement).disabled).toBe(true);
});

// A-U3: a candidate without any mounted source gets the same explanation.
it("U-3 without any mounted source explains the fix and offers no Create", async () => {
  const { card } = await setup([]);
  expect(within(card).getByRole("status").textContent?.includes("Mount the directory of the file provider")).toBe(true);
  expect(within(card).queryByRole("combobox", { name: "Configuration source" }) === null).toBe(true);
  expect((within(card).getByRole("button", { name: "Register instance" }) as HTMLButtonElement).disabled).toBe(true);
});

it("U-6 a held discovery answer from an earlier request never replaces the current one", async () => {
  const f = fixture();
  f.state.instances = [];
  const raw = f.fetcher.getMockImplementation()!;
  let release!: () => void;
  const held = new Promise<void>(resolve => (release = resolve));
  let calls = 0;
  f.fetcher.mockImplementation(async (input, init) => {
    if (!String(input).includes("/discover")) return raw(input, init);
    calls++;
    if (calls === 1) {
      await held;
      f.state.discovery = [{ identity: { ...instance.identity, containerName: "stale-proxy" }, kind: "traefik", image: "traefik:v3.7.16", sources: [config] }];
    } else f.state.discovery = candidate([config]);
    return raw(input, init);
  });
  vi.stubGlobal("fetch", f.fetcher);
  render(
    <Providers>
      <ProxyPage />
    </Providers>,
  );
  await screen.findByRole("heading", { name: "Set up proxy" });
  const user = userEvent.setup();
  screen.getByRole("combobox", { name: "Host" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: "Example host" }));
  await waitFor(() => expect(calls).toBe(1));
  await user.click(screen.getByRole("button", { name: "Discover containers" }));
  await screen.findByRole("heading", { name: instance.identity.containerName });
  release();
  await new Promise(resolve => setTimeout(resolve, 50));
  expect(screen.queryByRole("heading", { name: "stale-proxy" }) === null).toBe(true);
  expect(screen.getByRole("heading", { name: instance.identity.containerName }) !== null).toBe(true);
});
