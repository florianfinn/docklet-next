import { cleanup, waitFor } from "@testing-library/react";
import { Profiler } from "react";
import type { RouteObject } from "react-router";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/runtime", () => import("./runtime"));

import { routes } from "@/app/router";
import { renderRoutes } from "@/test/render";
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
    expect(errors).toEqual([]);
  } finally {
    error.mockRestore();
  }
}, 9000);
