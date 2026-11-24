// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original busy feedback and navigation lifetime proofs.

import { act, fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { toast } from "sonner";
import { renderWithApp } from "@/test/render";
import { CheckUpdatesButton, type CheckFeedback } from "./check-updates-button";

it("keeps a visible check busy until the result and reports the checked version", async () => {
  const success = vi.spyOn(toast, "success").mockImplementation(() => "example");
  let finish!: (v: CheckFeedback) => void;
  const pending = new Promise<CheckFeedback>((resolve) => { finish = resolve; });
  renderWithApp(<CheckUpdatesButton owner="container-a" check={() => pending} />);
  fireEvent.click(screen.getByRole("button", { name: "Nach Updates suchen" }));
  expect(screen.getByRole("button").getAttribute("aria-busy")).toBe("true");
  await act(async () => finish({ hasUpdate: true, version: "2.0.0" }));
  expect(success).toHaveBeenCalledWith("Update verfügbar: 2.0.0");
  expect(screen.getByRole("button").getAttribute("aria-busy")).toBe("false"); success.mockRestore();
});
it("does not publish a late result after navigation changes the request owner", async () => {
  const success = vi.spyOn(toast, "success").mockImplementation(() => "example");
  let finish!: (v: CheckFeedback) => void;
  const pending = new Promise<CheckFeedback>((resolve) => { finish = resolve; });
  const view = renderWithApp(<CheckUpdatesButton owner="container-a" check={() => pending} />);
  fireEvent.click(screen.getByRole("button", { name: "Nach Updates suchen" }));
  view.unmount();
  await act(async () => finish({ hasUpdate: true, version: "2.0.0" }));
  expect(success).not.toHaveBeenCalled(); success.mockRestore();
});
