// Source: docklet-next/issues/549
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original accessible-name guard for the shared step rail.

import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { StepRail } from "./step-rail";

it("names every step button by number and title, also for completed steps on phones", () => {
  const steps = ["Domain", "Destination", "TLS", "Handling"].map((title, i) => ({ id: String(i), title }));
  render(<StepRail label="Route editor steps" steps={steps} current="3" reachable={3} onSelect={() => {}} />);
  const names = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
  expect(names).toEqual(["1. Domain", "2. Destination", "3. TLS", "4. Handling"]);
});
