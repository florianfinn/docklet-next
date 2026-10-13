import { act, fireEvent, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ActionRunner } from "@/data/demo/action-runner";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { composeScript } from "@/data/demo/run-output";
import { simulationStore } from "@/data/demo/simulation";
import { setUiStyle, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { Toaster } from "@/shared/ui/sonner";
import { renderWithApp } from "@/test/render";

import { ActionToasts } from "./ActionToasts";
import { outputText } from "./run-output-text";

beforeEach(() => {
  vi.useFakeTimers();
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 1 }));
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  simulationStore.reset();
  uiStyleStore.reset();
  localStorage.clear();
});

const target = { project: "shop", services: [{ name: "web", image: "nginx:1.27" }], hostName: "Atlas" };

describe("raw action output", () => {
  it("arrives line by line while a step runs, in chronological order, and ends with a diagnosis on failure", async () => {
    const runner = new ActionRunner([]);
    const id = runner.start({
      kind: "redeploy",
      title: "Redeploy · Shop",
      target: "Shop · Atlas",
      steps: [
        { key: "validate", label: "Prüfung", ms: 1000 },
        { key: "pull", label: "Image-Pull", ms: 3000 },
      ],
      output: composeScript(target),
      failAt: "pull",
      failMessage: "Registry nicht erreichbar",
    });
    const run = () => runner.list().find((r) => r.id === id)!;
    expect(run().output.map((l) => l.stream)).toEqual(["note"]);
    await vi.advanceTimersByTimeAsync(1100);
    const afterValidate = run().output.length;
    await vi.advanceTimersByTimeAsync(1000);
    // Part of the pull output exists before the step ends.
    expect(run().status === "running" && run().output.length > afterValidate).toBe(true);
    await vi.advanceTimersByTimeAsync(3000);
    const out = run().output;
    expect(run().status).toBe("failed");
    expect(out.some((l) => l.stream === "cmd" && l.text === "docker compose --project-name shop pull web")).toBe(true);
    expect(out.filter((l) => l.level === "error").map((l) => l.text).at(-1)).toBe("Registry nicht erreichbar");
    expect(out.every((l, i) => i === 0 || out[i - 1]!.at <= l.at)).toBe(true);
    // Plain-text export keeps timestamps and streams.
    expect(outputText(run()).split("\n").slice(1).every((l) => /^\d{4}-\d{2}-\d{2}T.+ \[(cmd|stdout|stderr|note)\]/.test(l))).toBe(true);
  });

  it("gives seeded deploy history the same Compose output, timed by its steps", async () => {
    const adapter = createDemoAdapter();
    const craft = (await adapter.listActions()).find((r) => r.id === "run-craft-1")!;
    expect(craft.output.some((l) => /docker compose --project-name craft pull minecraft/.test(l.text))).toBe(true);
    expect(craft.output.filter((l) => l.level === "error").length).toBeGreaterThan(0);
    expect(craft.output.every((l) => l.at >= craft.startedAt)).toBe(true);
  });
});

describe("action toast lifetime", () => {
  it("shows progress without a countdown while running and closes itself when the lifetime bar ends", async () => {
    setUiStyle({ toastScale: 1.5 });
    const adapter = createDemoAdapter();
    renderWithApp(
      <>
        <ActionToasts />
        <Toaster />
      </>,
      adapter,
    );
    await act(async () => {
      await adapter.checkUpdates({ stackId: "cloud" });
      await vi.advanceTimersByTimeAsync(50);
    });
    const el = document.querySelector<HTMLElement>('[data-slot="action-toast"]');
    expect(screen.getByText("Update-Prüfung · Cloud") !== null && el?.dataset.status === "running" && el.querySelector('[data-slot="toast-life"]') === null).toBe(true);
    // Wide toast: the slim segment bar with one named segment per step, no step boxes.
    expect(el?.dataset.detail === "full" && el.querySelectorAll('[data-slot="step-track"] > li').length === 3).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    const life = document.querySelector<HTMLElement>('[data-slot="toast-life"]');
    expect([document.querySelector<HTMLElement>('[data-slot="action-toast"]')?.dataset.status, life?.style.getPropertyValue("--life")]).toEqual(["succeeded", "8000ms"]);
    // The real dismissal is the end of the lifetime animation (paused by CSS on hover or focus).
    const dismiss = vi.spyOn(toast, "dismiss");
    await act(async () => {
      fireEvent.animationEnd(life!, { animationName: "toast-life" });
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(dismiss).toHaveBeenCalledTimes(1);
  });
});
