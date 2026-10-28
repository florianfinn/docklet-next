import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Exercise the actual production entry while the rest of the suite keeps its demo runtime.
vi.mock("@/runtime", () => vi.importActual("./runtime"));

import { routes } from "@/app/router";
import { designStore, tokenPreviewStore } from "@/shared/theme/design-tokens";
import { favoriteStore } from "@/shared/preferences/favorite-code";
import { helpStore } from "@/shared/preferences/help-store";
import { renderRoutes } from "@/test/render";
import { createAdapter } from "./runtime";

beforeEach(() => vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
  const path = new URL(String(input), "http://localhost").pathname;
  if (path === "/api/auth/mode") return Response.json({ success: true, data: { mode: "none" } });
  if (path === "/api/auth/me") return Response.json({ success: true, data: { id: "admin", username: "admin", requiresPasswordChange: false } });
  if (path === "/api/environments/0/settings") return Response.json([{ key: "setupCompleted", value: "true" }]);
  return new Response(JSON.stringify({ code: "offline" }), { status: 503 });
})));

afterEach(() => {
  designStore.reset();
  tokenPreviewStore.set(null);
  favoriteStore.reset();
  helpStore.reset();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe("production without a backend", () => {
  it("fails reads and writes and never emits simulated streams", async () => {
    const adapter = createAdapter();
    expect(adapter.kind).toBe("http");
    await expect(adapter.listHosts()).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(adapter.getSettings()).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(adapter.runScan()).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    const listener = vi.fn();
    const stop = adapter.subscribeActions(listener);
    stop();
    expect(listener).not.toHaveBeenCalled();
  });

  it.each(["local", "proxy", "none"])("cannot enable the login simulation through variant=%s", async (variant) => {
    renderRoutes(routes, [`/login?variant=${variant}`], createAdapter());
    await waitFor(() => expect(document.querySelector('[data-slot="login-demo-controls"]') === null).toBe(true));
    expect(document.querySelector('[data-slot="login-demo-controls"]') === null).toBe(true);
    expect(screen.queryByRole("textbox")).toBe(null);
    fireEvent.keyDown(window, { altKey: true, shiftKey: true, code: "KeyD" });
    expect(document.querySelector('[data-layer="design-lab"]') === null).toBe(true);
  });

  it("saves appearance and imports a design code without mounting the lab or loading hub settings", async () => {
    const user = userEvent.setup();
    renderRoutes(routes, ["/settings/appearance?variant=local"], createAdapter());
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
    await act(() => router.navigate("/settings/help"));
    const off = await screen.findByRole("radio", { name: "Aus" });
    await userEvent.click(off);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(helpStore.get().mode).toBe("off"));
  });
});

it("maps advertised production scan operations to live responses without demo content", async () => {
  const { securityFixture } = await import("./data/http/vulnerabilities.test-support");
  const f = securityFixture(), adapter = createAdapter({ fetch: f.fetcher as typeof fetch, eventSource: () => f.source });
  expect((await adapter.getCapability("getSecurity")).available).toBe(true);
  const data = await adapter.getSecurity();
  expect(data.findings.every((finding) => finding.imageId === "sha256:user")).toBe(true);
  expect(data.images.some((image) => image.id === "second~sha256:user")).toBe(true);
  expect(await adapter.runScan({ hostIds: ["first"] })).toBe("scan-active-first");
});
