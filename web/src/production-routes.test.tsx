import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Profiler } from "react";
import type { RouteObject } from "react-router";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => import("./runtime"));

import { routes } from "@/app/router";
import { renderRoutes, renderWithApp } from "@/test/render";
import { ImportSourceDialog } from "@/features/stacks/editor/ImportSourceDialog";
import { createAdapter } from "./runtime";

const paths = [
  "/dashboard", "/hosts", "/hosts/new", "/stacks", "/stacks/new", "/stacks/missing",
  "/containers", "/containers/missing", "/images", "/volumes", "/volumes/missing",
  "/backups", "/networks", "/security", "/profile", "/setup", "/login",
  "/settings/general", "/settings/appearance", "/settings/activity", "/settings/hosts",
  "/settings/notifications", "/settings/auth", "/missing",
];

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
    renderRoutes(bounded(routes), [path], createAdapter());
    await waitFor(() => expect(document.body.textContent?.trim().length).toBeGreaterThan(20));
    await new Promise((resolve) => setTimeout(resolve, path === "/stacks/new" ? 5000 : 750));
    expect(commits).toBeLessThanOrEqual(40);
    expect(timers.mock.calls.length).toBeLessThan(200);
    expect(errors).toEqual([]);
  } finally {
    error.mockRestore();
    timers.mockRestore();
  }
}, 9000);

it("handles a rejected source import with the product disconnected adapter", async () => {
  const errors: string[] = [];
  const error = vi.spyOn(console, "error").mockImplementation((...args) => errors.push(args.map(String).join(" ")));
  try {
    renderWithApp(<ImportSourceDialog open onOpenChange={() => undefined} onApply={() => undefined} />, createAdapter());
    fireEvent.click(await screen.findByRole("button", { name: "github.com/example/notes-app" }));
    expect(await screen.findByText("Quelle nicht verfügbar. Später erneut versuchen.")).toBeTruthy();
    expect(errors).toEqual([]);
  } finally {
    error.mockRestore();
  }
});
