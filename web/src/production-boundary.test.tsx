import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

// Exercise the actual production entry while the rest of the suite keeps its demo runtime.
vi.mock("@/runtime", () => import("./runtime"));

import { routes } from "@/app/router";
import { BACKEND_DISCONNECTED } from "@/data/disconnected-adapter";
import { designStore, tokenPreviewStore } from "@/shared/theme/design-tokens";
import { favoriteStore } from "@/shared/preferences/favorite-code";
import { helpStore } from "@/shared/preferences/help-store";
import { renderRoutes } from "@/test/render";
import { createAdapter } from "./runtime";

afterEach(() => {
  designStore.reset();
  tokenPreviewStore.set(null);
  favoriteStore.reset();
  helpStore.reset();
  localStorage.clear();
});

describe("production without a backend", () => {
  it("fails reads and writes and never emits simulated streams", async () => {
    const adapter = createAdapter();
    expect(adapter.kind).toBe("disconnected");
    await expect(adapter.listHosts()).rejects.toThrow(BACKEND_DISCONNECTED);
    await expect(adapter.getSettings()).rejects.toThrow(BACKEND_DISCONNECTED);
    await expect(adapter.runScan()).rejects.toThrow(BACKEND_DISCONNECTED);
    const listener = vi.fn();
    const stop = adapter.subscribeActions(listener);
    stop();
    expect(listener).not.toHaveBeenCalled();
  });

  it.each(["local", "proxy", "none"])("cannot enable the login simulation through variant=%s", async (variant) => {
    renderRoutes(routes, [`/login?variant=${variant}`], createAdapter());
    await screen.findByText("Kein Backend verbunden");
    expect(document.querySelector('[data-slot="login-demo-controls"]') === null).toBe(true);
    expect(screen.queryByRole("textbox")).toBe(null);
    fireEvent.keyDown(window, { altKey: true, shiftKey: true, code: "KeyD" });
    expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
  });

  it("saves appearance and imports a design code without mounting the lab or loading hub settings", async () => {
    const user = userEvent.setup();
    renderRoutes(routes, ["/settings/appearance?variant=local"], createAdapter());
    await screen.findByText("Kein Backend verbunden");
    await user.click(await screen.findByRole("radio", { name: "Hell" }));
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(designStore.get().tokens.theme).toBe("light"));
    await user.click(screen.getByRole("button", { name: "Aktuellen Code erzeugen" }));
    const code = (document.querySelector('[data-slot="design-code"]') as HTMLTextAreaElement).value;
    await user.click(screen.getByRole("radio", { name: "Dunkel" }));
    await user.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(designStore.get().tokens.theme).toBe("dark"));
    fireEvent.change(screen.getByRole("textbox", { name: "Code einfügen" }), { target: { value: code } });
    await user.click(screen.getByRole("button", { name: "Code anwenden" }));
    expect(designStore.get().tokens.theme).toBe("light");
    expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
    fireEvent.keyDown(window, { altKey: true, shiftKey: true, code: "KeyD" });
    expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
    expect(screen.queryByRole("button", { name: "Design-Labor öffnen" })).toBe(null);
  });

  it("keeps explanation preferences available after the settings query fails", async () => {
    const { router } = renderRoutes(routes, ["/settings/general"], createAdapter());
    await screen.findByText("Kein Backend verbunden");
    await act(() => router.navigate("/settings/help"));
    const off = await screen.findByRole("radio", { name: "Aus" });
    await userEvent.click(off);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(helpStore.get().mode).toBe("off"));
  });
});
