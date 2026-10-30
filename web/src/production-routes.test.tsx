import { cleanup, fireEvent, screen } from "@testing-library/react";
import { Profiler } from "react";
import { matchRoutes, type RouteObject } from "react-router";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => vi.importActual("./runtime"));

import { NAV } from "@/app/shell/nav-items";
import type { renderRoutes, renderWithApp } from "@/test/render";
import type { ImportSourceDialog } from "@/features/stacks/editor/ImportSourceDialog";
import type { createAdapter } from "./runtime";

const paths = [
  "/dashboard", "/hosts", "/hosts/new", "/stacks", "/stacks/new", "/stacks/missing",
  "/containers", "/containers/missing", "/images", "/volumes", "/volumes/missing",
  "/backups", "/networks", "/security", "/profile", "/setup", "/login",
  "/settings/general", "/settings/appearance", "/settings/activity", "/settings/hosts",
  "/settings/notifications", "/settings/autoHeal", "/settings/auth", "/missing", "/preview/unknown",
];

let routes: RouteObject[];
let navigation: typeof NAV;
let renderProductRoutes: typeof renderRoutes;
let renderProductComponent: typeof renderWithApp;
let ProductImportSourceDialog: typeof ImportSourceDialog;
let productAdapter: typeof createAdapter;

// Cold router imports use a generous hang timeout so shared CI load does not fail setup.
beforeAll(async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ code: "offline" }), { status: 503 })));
  vi.stubEnv("DEV", false);
  vi.resetModules();
  ({ routes, navigation } = await import("@/app/router"));
  ({ renderRoutes: renderProductRoutes, renderWithApp: renderProductComponent } = await import("@/test/render"));
  ({ ImportSourceDialog: ProductImportSourceDialog } = await import("@/features/stacks/editor/ImportSourceDialog"));
  ({ createAdapter: productAdapter } = await import("./runtime"));
}, 60_000);

afterAll(() => { vi.unstubAllEnvs(); vi.unstubAllGlobals(); });
afterEach(() => cleanup());

it.each(paths)("settles with the product HTTP adapter when the backend is unavailable: %s", async (path) => {
  let commits = 0;
  const errors: string[] = [];
  const error = vi.spyOn(console, "error").mockImplementation((...args) => errors.push(args.map(String).join(" ")));
  const timers = vi.spyOn(globalThis, "setTimeout");
  const bounded = (items: RouteObject[]): RouteObject[] => items.map((route): RouteObject => {
    const element = route.element ? <Profiler id={route.path ?? "shell"} onRender={() => {
      commits += 1;
      if (commits > 40) throw new Error(`Runaway route render: ${path}`);
    }}>{route.element}</Profiler> : route.element;
    return route.index ? { ...route, element } : { ...route, element, children: route.children ? bounded(route.children) : undefined };
  });
  try {
    renderProductRoutes(bounded(routes), [path], productAdapter());
    await screen.findByText(path === "/setup" ? "Der Status der Ersteinrichtung konnte nicht geprüft werden." : "Die Anmeldeart konnte nicht geladen werden. Bitte erneut versuchen.");
    expect(commits).toBeLessThanOrEqual(40);
    expect(timers.mock.calls.length).toBeLessThan(200);
    expect(errors).toEqual([]);
    expect(document.body.textContent).not.toMatch(/Demo artifact|Fehlersimulation|React-Prototyp|Alex Beispiel|alex@example\.com/);
    if (path === "/hosts") expect(document.querySelector('a[href="/hosts/new"]')).toBeNull();
  } finally {
    error.mockRestore();
    timers.mockRestore();
  }
}, 9000);

it("keeps protected navigation hidden while the sign-in policy is unavailable", async () => {
  renderProductRoutes(routes, ["/dashboard"], productAdapter());
  await screen.findByText("Die Anmeldeart konnte nicht geladen werden. Bitte erneut versuchen.");
  expect(document.querySelector('[data-slot="sidebar"]')).toBeNull();
  const links = navigation.flatMap((group) => group.items);
  expect(links.length).toBeGreaterThan(0);
  expect(links.some((item) => item.to.startsWith("/preview/"))).toBe(false);
  for (const item of links) {
    const matches = matchRoutes(routes, item.to);
    expect(matches?.length).toBeGreaterThan(0);
    expect(matches?.some(({ route }) => route.path === "*")).toBe(false);
  }
});

it("keeps unmapped source imports unavailable with the product HTTP adapter", async () => {
  const errors: string[] = [];
  const error = vi.spyOn(console, "error").mockImplementation((...args) => errors.push(args.map(String).join(" ")));
  try {
    renderProductComponent(<ProductImportSourceDialog open onOpenChange={() => undefined} onApply={() => undefined} />, productAdapter());
    const example = await screen.findByRole("button", { name: "github.com/example/notes-app" });
    expect(example.hasAttribute("disabled")).toBe(true);
    fireEvent.click(example);
    expect(await screen.findByRole("status")).toBeTruthy();
    expect(errors).toEqual([]);
  } finally {
    error.mockRestore();
  }
});
