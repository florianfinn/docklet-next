// Source: docklet-next/issues/210
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original localized container command fallback regression.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import { useContainerCommands } from "./use-container-commands";

afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });
it.each([
  ["en-US", "The request could not be completed."],
  ["de", "Die Anfrage konnte nicht abgeschlossen werden."],
] as const)("uses the translated generic container fallback in %s", async (locale, expected) => {
  setAppLocale(locale);
  const error = vi.spyOn(toast, "error");
  function Probe() { const cmd = useContainerCommands(); return <button onClick={() => void cmd.setOrigin("example", "standalone")}>Change origin</button>; }
  renderWithApp(<Probe />, { ...createDemoAdapter(), setContainerOrigin: async () => { throw new Error("synthetic internal exception"); } });
  await userEvent.click(screen.getByRole("button", { name: "Change origin" }));
  await waitFor(() => expect(error).toHaveBeenCalled());
  expect(error).toHaveBeenCalledWith(expected);
  expect(error.mock.calls.some(([message]) => String(message).includes("synthetic internal exception"))).toBe(false);
});
