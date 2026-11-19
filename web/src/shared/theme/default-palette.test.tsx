// Source: docklet-next/issues/521
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original first-run default palette test.

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { DEFAULT_TOKENS } from "@/shared/theme/design-tokens";
import { renderWithApp } from "@/test/render";

import { PaletteEditor } from "./PaletteEditor";

afterEach(() => { cleanup(); localStorage.clear(); });

it("shows Purple & Cyan selected on a fresh browser", async () => {
  renderWithApp(<PaletteEditor value={DEFAULT_TOKENS.palette} onChange={() => undefined} theme={DEFAULT_TOKENS.theme} idPrefix="fresh" />);
  const nebula = await screen.findByRole("radio", { name: /Lila & Cyan/ });
  expect(nebula.getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("radio", { name: /Arcane Violett/ }).getAttribute("aria-checked")).toBe("false");
});
