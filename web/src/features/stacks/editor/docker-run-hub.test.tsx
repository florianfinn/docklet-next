// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original hub docker run conversion tests for issue #139.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";

import { DockerRunDialog } from "./DockerRunDialog";
import { hubConversion } from "./hub-conversion";

function hubAdapter(convert: DockletAdapter["convertDockerRun"]): DockletAdapter {
  return { ...createDemoAdapter(), kind: "http", convertDockerRun: convert };
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
    expect(onAdd).toHaveBeenCalledWith("web-2", { image: "nginx:1.29" }, "A=1");
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
});
