import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { GLOW_SELECTOR, glowCandidates, glowTarget } from "@/shared/lib/pointer-glow";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { mockWideWorkspace } from "@/test/layout";
import { renderRoutes } from "@/test/render";

import StackPage from "../detail/StackPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  mockWideWorkspace();
});

afterEach(() => {
  vi.restoreAllMocks();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

describe("Easy replaces the editor area", () => {
  it("renders only the full-width wizard: no file tree, no editor, no live YAML preview", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const modes = screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" });
    await userEvent.click(within(modes).getByRole("radio", { name: "Easy" }));

    const wizard = screen.getByRole("region", { name: "Geführter Entwurf" });
    expect(wizard.dataset.slot).toBe("easy-wizard");
    expect(document.querySelector('[data-slot="workspace"]') === null).toBe(true);
    expect(document.querySelector('[data-slot="file-tree"]') === null).toBe(true);
    expect(document.querySelector('[data-slot="code-editor"]') === null).toBe(true);
    expect(screen.queryByText("compose.yaml · live") === null).toBe(true);
    expect(screen.queryByRole("navigation", { name: "Dateien" }) === null).toBe(true);

    // Service sections wrap or stand vertically; they never scroll sideways.
    await userEvent.click(within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getByRole("button", { name: /Services/ }));
    const sections = await screen.findByRole("tablist", { name: /^Bereiche von/ });
    expect(sections.className.includes("overflow-x-auto")).toBe(false);
    expect(sections.className.includes("flex-wrap")).toBe(true);
    expect(within(sections).getAllByRole("tab").length).toBe(7);

    // Back in Advanced the shared workspace with tree and CodeMirror returns.
    await userEvent.click(within(modes).getByRole("radio", { name: "Advanced" }));
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    expect(document.querySelector('[data-slot="workspace"] [data-slot="file-tree"]') !== null).toBe(true);
  });

  it("offers a details aside or a second file in Advanced; the second file edits the same draft", async () => {
    preferencesStore.set((p) => ({ ...p, workspaceAside: "split" }));
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const aside = document.querySelector('[data-slot="workspace-aside"]') as HTMLElement;
    expect(within(aside).getByRole("textbox", { name: "Inhalt von .env" }) !== null).toBe(true);
    expect(document.querySelectorAll('[data-slot="code-editor"]').length).toBe(2);

    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Neben dem Editor" })).getByRole("radio", { name: "Details" }));
    expect(preferencesStore.get().workspaceAside).toBe("details");
    const details = document.querySelector('[data-slot="workspace-aside"]') as HTMLElement;
    expect(within(details).getByText("Keine Fehler oder Hinweise.") !== null).toBe(true);
    expect(within(details).getByRole("link", { name: "media_config" }).getAttribute("href")).toBe("/volumes/vol-atlas-media_media_config?tab=files");
    expect(document.querySelectorAll('[data-slot="code-editor"]').length).toBe(1);
  });

  it("renders no aside and no aside choice while the workspace is narrow", async () => {
    vi.restoreAllMocks();
    preferencesStore.set((p) => ({ ...p, workspaceAside: "split" }));
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/media?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    expect(document.querySelector('[data-slot="workspace-aside"]') === null && screen.queryByRole("radiogroup", { name: "Neben dem Editor" }) === null).toBe(true);
    expect(document.querySelectorAll('[data-slot="code-editor"]').length).toBe(1);
  });
});

describe("design colours and edge light in Easy and Advanced", () => {
  it("registers frame, step cards, step rows and service rows for the edge light; selections use the control token", async () => {
    renderRoutes([{ path: "/stacks/:stackId", element: <StackPage /> }], ["/stacks/paperless?tab=editor"]);
    await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" });
    const modes = screen.getByRole("radiogroup", { name: "Bearbeitungsmodus" });
    // Advanced: the workspace frame carries the light; the editor itself is no glow target.
    expect(document.querySelector('[data-slot="workspace"]')?.matches(GLOW_SELECTOR)).toBe(true);
    expect(glowTarget(document.querySelector(".cm-content"))?.dataset.slot).toBe("workspace");
    await userEvent.click(within(modes).getByRole("radio", { name: "Easy" }));
    const wizard = screen.getByRole("region", { name: "Geführter Entwurf" });
    const steps = within(screen.getByRole("list", { name: "Schritte des Entwurfs" })).getAllByRole("button");
    expect([wizard.matches(GLOW_SELECTOR), steps.every((b) => b.matches(GLOW_SELECTOR)), wizard.querySelector('[data-slot="card"]') !== null]).toEqual([true, true, true]);
    // The active step (an existing stack opens on Services) uses the control token of the design, not a fixed colour.
    const current = steps.find((b) => b.getAttribute("aria-current") === "step");
    expect([steps.indexOf(current!), current?.className.includes("bg-control/15"), steps[0]?.className.includes("bg-control/15")]).toEqual([1, true, false]);
    // A neighbouring step never takes the hovered step's light.
    expect(glowCandidates([steps[0]!], [steps[1]!])).toEqual([steps[0]]);

    const services = screen.getAllByRole("list", { name: "Services" })[0]!;
    const rows = [...services.querySelectorAll<HTMLElement>('[data-glow="row"]')];
    expect([rows.map((r) => r.querySelector("span")?.textContent), rows.every((r) => r.matches(GLOW_SELECTOR))]).toEqual([["webserver", "db", "broker"], true]);
    const active = within(screen.getByRole("tablist", { name: /^Bereiche von/ })).getByRole("tab", { selected: true });
    expect(active.className.includes("data-[state=active]:bg-control/15")).toBe(true);
    // Editing in the wizard still reaches the compose text (data and validation untouched).
    await userEvent.click(within(modes).getByRole("radio", { name: "Advanced" }));
    expect((await screen.findByRole("textbox", { name: "Inhalt von compose.yaml" })).textContent?.includes("depends_on: [webserver]")).toBe(true);
  });
});

