import { act, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, describe, expect, it } from "vitest";

import { activityStore } from "@/features/activity/activity-store";
import css from "@/styles/notifications.css?raw";
import { renderWithApp } from "@/test/render";

import { AppToaster } from "./AppToaster";
import { TOAST_MOBILE_OFFSET, TOAST_OFFSET } from "./toast-corner";

afterEach(() => {
  act(() => void toast.dismiss());
  activityStore.reset();
  document.body.innerHTML = "";
});

describe("toast corner", () => {
  it("keeps one fixed top-right offset inside the safe area, whatever the page actions or open sheets do", async () => {
    // A tall action row and an open action centre used to push the column down or sideways.
    const actions = document.body.appendChild(document.createElement("div"));
    actions.setAttribute("data-page-actions", "");
    actions.getBoundingClientRect = () => DOMRect.fromRect({ x: 900, y: 200, width: 400, height: 60 });
    act(() => activityStore.set((s) => ({ ...s, open: true })));
    renderWithApp(<AppToaster />);
    act(() => void toast.info("Hinweis"));
    await screen.findByText("Hinweis");
    const list = document.querySelector<HTMLElement>("[data-sonner-toaster]")!;
    expect([list.dataset.yPosition, list.dataset.xPosition, list.style.getPropertyValue("--offset-top"), list.style.getPropertyValue("--offset-right")]).toEqual([
      "top",
      "right",
      TOAST_OFFSET.top,
      TOAST_OFFSET.right,
    ]);
    expect(TOAST_OFFSET.top.includes("safe-area-inset-top")).toBe(true);
  });

  it("sits in the true viewport corner: the same small gap at the top as at the right, no header clearance", () => {
    expect([TOAST_OFFSET.top, TOAST_OFFSET.right]).toEqual(["calc(16px + env(safe-area-inset-top, 0px))", "calc(16px + env(safe-area-inset-right, 0px))"]);
    expect([TOAST_MOBILE_OFFSET.top, TOAST_MOBILE_OFFSET.right, TOAST_MOBILE_OFFSET.left]).toEqual([
      "calc(8px + env(safe-area-inset-top, 0px))",
      "calc(8px + env(safe-area-inset-right, 0px))",
      "calc(8px + env(safe-area-inset-left, 0px))",
    ]);
  });

  it("never draws the browser outline around a focused toast; only the card ring", () => {
    expect(/\[data-sonner-toast\]:focus\s*\{\s*outline: none;/.test(css)).toBe(true);
  });
});
