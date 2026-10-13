import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Slider } from "./slider";

describe("slider", () => {
  it("names each thumb, which carries role=slider, instead of the unlabelled root", () => {
    render(
      <>
        <Slider aria-label="Reichweite innen" aria-valuetext="32 px" value={[32]} min={8} max={64} />
        <Slider aria-label="Bereich" value={[2, 8]} min={0} max={10} />
      </>,
    );
    const inner = screen.getByRole("slider", { name: "Reichweite innen" });
    expect([inner.getAttribute("aria-valuetext"), inner.getAttribute("aria-valuenow")]).toEqual(["32 px", "32"]);
    expect(screen.getAllByRole("slider").map((s) => s.getAttribute("aria-label"))).toEqual(["Reichweite innen", "Bereich 1", "Bereich 2"]);
    expect(document.querySelector('[data-slot="slider"][aria-label]') === null).toBe(true);
  });
});
