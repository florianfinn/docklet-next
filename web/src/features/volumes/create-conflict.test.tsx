// Source: docklet-next/issues/221
// Revision: 75e73d5c4ffcd78b47ebaa166579079c02f70c8f
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original HTTP create conflict to inline feedback and retained draft coverage.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { createVolumeCommands } from "@/data/http/volume-commands";
import { renderRoutes } from "@/test/render";
import { CreateVolumeDialog } from "./CreateVolumeDialog";

afterEach(() => { simulationStore.reset(); localStorage.clear(); setAppLocale("de"); });

it.each(["en-US", "de"] as const)("shows the API name clash inline and keeps the create draft (%s)", async (locale) => {
  setAppLocale(locale);
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const base = createDemoAdapter();
  const hosts = await base.listHosts();
  const fetch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ status: 409, title: "Conflict", detail: "A volume with this name already exists", code: "volume.exists" }), { status: 409, headers: { "Content-Type": "application/problem+json" } }));
  const commands = createVolumeCommands({ baseUrl: "/api", client: { fetch, token: async () => "example" }, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), changed: vi.fn() });
  const onOpenChange = vi.fn();
  const { router } = renderRoutes([{ path: "/volumes", element: <CreateVolumeDialog open onOpenChange={onOpenChange} hosts={hosts} volumes={[]} defaultHostId="atlas" /> }], ["/volumes"], { ...base, kind: "http", createVolume: commands.createVolume });
  const name = screen.getByRole("textbox", { name: "Name" });
  const labels = screen.getByRole("textbox", { name: /Labels/ });
  await userEvent.type(name, "occupied");
  await userEvent.type(labels, "example.owner=draft");
  const submit = screen.getByRole("button", { name: locale === "en-US" ? "Create" : "Anlegen" });
  await waitFor(() => expect(submit.hasAttribute("disabled")).toBe(false));
  await userEvent.click(submit);
  const dialog = screen.getByRole("dialog");
  expect((await within(dialog).findByRole("alert")).textContent).toContain(locale === "en-US" ? "Name already exists on this host." : "Name existiert bereits auf diesem Host.");
  expect((name as HTMLInputElement).value).toBe("occupied");
  expect((labels as HTMLTextAreaElement).value).toBe("example.owner=draft");
  expect(onOpenChange).not.toHaveBeenCalled();
  expect(router.state.location.pathname).toBe("/volumes");
  expect(fetch).toHaveBeenCalledOnce();
  expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body))).toMatchObject({ name: "occupied", labels: { "example.owner": "draft" } });
});
