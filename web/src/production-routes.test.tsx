import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Profiler } from "react";
import { matchRoutes, type RouteObject } from "react-router";
import { afterAll, afterEach, beforeAll, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => import("./runtime"));

import { NAV } from "@/app/shell/nav-items";
import type { renderRoutes, renderWithApp } from "@/test/render";
import type { ImportSourceDialog } from "@/features/stacks/editor/ImportSourceDialog";
import type { createAdapter } from "./runtime";

const paths = [
  "/dashboard", "/hosts", "/hosts/new", "/stacks", "/stacks/new", "/stacks/missing",
  "/containers", "/containers/missing", "/images", "/volumes", "/volumes/missing",
  "/backups", "/networks", "/security", "/profile", "/setup", "/login",
  "/settings/general", "/settings/appearance", "/settings/activity", "/settings/hosts",
  "/settings/notifications", "/settings/auth", "/missing", "/preview/unknown",
];

let routes: RouteObject[];
let navigation: typeof NAV;
let renderProductRoutes: typeof renderRoutes;
let renderProductComponent: typeof renderWithApp;
let ProductImportSourceDialog: typeof ImportSourceDialog;
let productAdapter: typeof createAdapter;

beforeAll(async () => {
  vi.stubEnv("DEV", false);
  vi.resetModules();
  ({ routes, navigation } = await import("@/app/router"));
  ({ renderRoutes: renderProductRoutes, renderWithApp: renderProductComponent } = await import("@/test/render"));
  ({ ImportSourceDialog: ProductImportSourceDialog } = await import("@/features/stacks/editor/ImportSourceDialog"));
  ({ createAdapter: productAdapter } = await import("./runtime"));
});

afterAll(() => vi.unstubAllEnvs());
afterEach(() => cleanup());

it.each(paths)("settles with the product disconnected adapter: %s", async (path) => {
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
    await waitFor(() => expect(document.body.textContent?.trim().length).toBeGreaterThan(20));
    await new Promise((resolve) => setTimeout(resolve, path === "/stacks/new" ? 5000 : 750));
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

it("shows only sidebar links backed by a production route", async () => {
  renderProductRoutes(routes, ["/dashboard"], productAdapter());
  await waitFor(() => expect(document.querySelector('[data-slot="sidebar"] a[href="/dashboard"]')).not.toBeNull());
  const links = navigation.flatMap((group) => group.items);
  expect(links.length).toBeGreaterThan(0);
  expect(links.some((item) => item.to.startsWith("/preview/"))).toBe(false);
  for (const item of links) {
    const matches = matchRoutes(routes, item.to);
    expect(matches?.length).toBeGreaterThan(0);
    expect(matches?.some(({ route }) => route.path === "*")).toBe(false);
  }
  const sidebarLinks = [...document.querySelectorAll<HTMLAnchorElement>('[data-slot="sidebar"] a[href^="/"]')];
  for (const link of sidebarLinks) {
    const matches = matchRoutes(routes, link.pathname);
    expect(matches?.length).toBeGreaterThan(0);
    expect(matches?.some(({ route }) => route.path === "*")).toBe(false);
  }
});

it("handles a rejected source import with the product disconnected adapter", async () => {
  const errors: string[] = [];
  const error = vi.spyOn(console, "error").mockImplementation((...args) => errors.push(args.map(String).join(" ")));
  try {
    renderProductComponent(<ProductImportSourceDialog open onOpenChange={() => undefined} onApply={() => undefined} />, productAdapter());
    fireEvent.click(await screen.findByRole("button", { name: "github.com/example/notes-app" }));
    expect(await screen.findByText("Quelle nicht verfügbar. Später erneut versuchen.")).toBeTruthy();
    expect(errors).toEqual([]);
  } finally {
    error.mockRestore();
  }
});
