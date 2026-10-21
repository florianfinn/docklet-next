// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original hub docker run conversion tests for issue #139.

import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";

import { DockerRunDialog } from "./DockerRunDialog";
import { useState } from "react";

/** Keeps one mounted dialog, as StackWorkspace does, and exposes its props as buttons. */
function Harness() {
  const [open, setOpen] = useState(true);
  const [hostId, setHostId] = useState("h1");
  return (
    <>
      <button type="button" onClick={() => setOpen((o) => !o)}>toggle-open</button>
      <button type="button" onClick={() => setHostId((h) => (h === "h1" ? "h2" : "h1"))}>toggle-host</button>
      <DockerRunDialog open={open} onOpenChange={setOpen} existing={[]} hostId={hostId} onAdd={() => {}} />
    </>
  );
}

type Converted = Awaited<ReturnType<DockletAdapter["convertDockerRun"]>>;
const NGINX: Converted = { compose: "services:\n  nginx:\n    image: nginx:1.29\n", env: "", serviceName: "nginx" };

/** One deferred hub request per call. */
function deferred() {
  const calls: { resolve: (v: Converted) => void; reject: (e: unknown) => void }[] = [];
  const convert = vi.fn(() => new Promise<Converted>((resolve, reject) => { calls.push({ resolve, reject }); }));
  return { convert, calls };
}
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

  for (const outcome of ["success", "failure"] as const) {
    it(`drops a ${outcome} that arrives after the dialog was closed and reopened`, async () => {
      const { convert, calls } = deferred();
      renderWithApp(<Harness />, hubAdapter(convert));
      await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
      await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
      await userEvent.click(screen.getByRole("button", { name: "Abbrechen" }));
      await act(async () => { if (outcome === "success") calls[0]!.resolve(NGINX); else calls[0]!.reject(new Error("late")); });
      fireEvent.click(screen.getByText("toggle-open"));
      expect((await screen.findByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
      expect(screen.queryByRole("alert") === null).toBe(true);
    });
  }

  it("drops a response after a host round trip", async () => {
    const { convert, calls } = deferred();
    renderWithApp(<Harness />, hubAdapter(convert));
    await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    fireEvent.click(screen.getByText("toggle-host"));
    fireEvent.click(screen.getByText("toggle-host"));
    await act(async () => { calls[0]!.resolve(NGINX); });
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("drops a pending response when the parent closes the dialog", async () => {
    const { convert, calls } = deferred();
    renderWithApp(<Harness />, hubAdapter(convert));
    await userEvent.type(screen.getByRole("textbox", { name: "Befehl" }), "docker run nginx:1.29");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    fireEvent.click(screen.getByText("toggle-open"));
    await act(async () => { calls[0]!.resolve(NGINX); });
    fireEvent.click(screen.getByText("toggle-open"));
    expect((await screen.findByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("does not settle an earlier request while a newer one is pending", async () => {
    const { convert, calls } = deferred();
    renderWithApp(<Harness />, hubAdapter(convert));
    const box = screen.getByRole("textbox", { name: "Befehl" });
    await userEvent.type(box, "docker run nginx:1.29");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    await userEvent.type(box, " ");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    expect(calls.length).toBe(2);
    await act(async () => { calls[0]!.resolve(NGINX); });
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => { calls[1]!.resolve(NGINX); });
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("drops a response after the command was edited back to the same text", async () => {
    const { convert, calls } = deferred();
    renderWithApp(<DockerRunDialog open onOpenChange={() => {}} existing={[]} hostId="h1" onAdd={() => {}} />, hubAdapter(convert));
    const box = screen.getByRole("textbox", { name: "Befehl" });
    await userEvent.type(box, "docker run nginx:1.29");
    await userEvent.click(screen.getByRole("button", { name: "Auf dem Hub umwandeln" }));
    await userEvent.type(box, "x");
    await userEvent.type(box, "{Backspace}");
    await act(async () => { calls[0]!.resolve(NGINX); });
    expect((screen.getByRole("button", { name: "Als Service hinzufügen" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
