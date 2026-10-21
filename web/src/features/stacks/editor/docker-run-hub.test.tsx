// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original hub docker run conversion tests for issue #139.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";

import { DockerRunDialog } from "./DockerRunDialog";
import { hubConversion } from "./hub-conversion";

function hubAdapter(convert: DockletAdapter["convertDockerRun"]): DockletAdapter {
  const getCapability: DockletAdapter["getCapability"] = async (operationId, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: true });
  return { ...createDemoAdapter(), kind: "http", getCapability, convertDockerRun: convert };
}

describe("docker run conversion on the hub", () => {
  it("converts only on request for the draft host and adds the hub service with its env", async () => {
    const convert = vi.fn(async () => ({ compose: "services:\n  web:\n    image: nginx:1.29\n", env: "A=1", serviceName: "web" }));
    const onAdd = vi.fn();
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={["web"]} hostId="h1" onAdd={onAdd} />, hubAdapter(convert));
    await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
    expect(convert).not.toHaveBeenCalled();
    const add = screen.getByRole("button", { name: "Als Service hinzufügen" });
    expect((add as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    expect(convert).toHaveBeenCalledWith("h1", "docker run nginx:1.29");
    await waitFor(() => expect((add as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(add);
    expect(onAdd).toHaveBeenCalledWith("web-2", { image: "nginx:1.29" }, "A=1", {});
  });

  it("distinguishes unparsable commands from hub failures and clears on edit", async () => {
    const convert = vi.fn<DockletAdapter["convertDockerRun"]>()
      .mockRejectedValueOnce(new AdapterOutcomeError({ kind: "error", code: "stack.convert-invalid", status: 400 }))
      .mockRejectedValueOnce(new AdapterOutcomeError({ kind: "error", code: "http.error", status: 500 }));
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={[]} hostId="h1" onAdd={() => {}} />, hubAdapter(convert));
    const box = screen.getByRole("textbox", { name: "Befehl" });
    await userEvent.type(box, "docker ps");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Befehl muss mit „docker run“ beginnen.");
    await userEvent.type(box, " -a");
    expect(screen.queryByRole("alert") === null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Der Hub konnte den Befehl nicht umwandeln");
  });

  it("keeps the local live preview in the demo", async () => {
    const convert = vi.fn();
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={[]} hostId="h1" onAdd={() => {}} />, { ...createDemoAdapter(), convertDockerRun: convert });
    await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
    expect(screen.queryByRole("button", { name: "Auf dem Hub umwandeln" }) === null).toBe(true);
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(false);
    expect(convert).not.toHaveBeenCalled();
  });

  it("rejects hub documents without the named service", () => {
    expect(hubConversion("services:\n  other: {}\n", "", "web", [])).toEqual({ ok: false, reason: "failed" });
  });

  it("ignores a hub response for a command that was edited meanwhile", async () => {
    let resolve: (value: Awaited<ReturnType<DockletAdapter["convertDockerRun"]>>) => void = () => {};
    const convert = vi.fn(() => new Promise<Awaited<ReturnType<DockletAdapter["convertDockerRun"]>>>((r) => { resolve = r; }));
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={[]} hostId="h1" onAdd={() => {}} />, hubAdapter(convert));
    const box = screen.getByRole("textbox", { name: "Befehl" });
    await userEvent.type(box, "docker run nginx:1.29");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    await userEvent.clear(box);
    await userEvent.type(box, "docker run redis:8");
    await act(async () => { resolve({ compose: "services:\n  nginx:\n    image: nginx:1.29\n", env: "", serviceName: "nginx" }); });
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("disables hub conversion when the host descriptor refuses it", async () => {
    const adapter = { ...hubAdapter(vi.fn()), getCapability: async (operationId: string, resourceRef = { kind: "hub", id: "0" }) => ({ operationId, resourceRef, available: false, disabledReasonCode: "capability.unavailable" }) };
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={[]} hostId="h1" onAdd={() => {}} />, adapter);
    await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
    await waitFor(() => expect((screen.getByRole("button", { name: "Auf dem Hub umwandeln" }) as HTMLButtonElement).disabled).toBe(true));
  });

  it("keeps top-level networks and volumes and refuses unsupported sections", () => {
    const compose = "services:\n  web:\n    image: nginx:1.29\n    networks: [example-net]\nnetworks:\n  example-net:\n    external: true\nvolumes:\n  example-data:\n    external: true\n";
    const result = hubConversion(compose, "", "web", []);
    expect(result.ok && result.declarations).toEqual({ networks: { "example-net": { external: true } }, volumes: { "example-data": { external: true } } });
    expect(hubConversion("services:\n  web: {}\nsecrets:\n  s: {}\n", "", "web", []).ok).toBe(false);
  });
});
