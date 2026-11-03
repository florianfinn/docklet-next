// Source: docklet-next/issues/340
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original guard that German state labels translate the Docker health state.

import { expect, it } from "vitest";

import { catalogs } from "./catalog";

// Texts that explain Docker's literal health value (Auto Heal, health checks, Docker output patterns) may name it.
const DOCKER_TERM = /^(help\.|editor\.|settings\.autoHeal\.|runMessages\.pattern\.)/;

it("translates the unhealthy state in German labels, counts and status texts", () => {
  const found: string[] = [];
  const walk = (value: unknown, path: string) => {
    if (typeof value === "string") {
      if (!DOCKER_TERM.test(path) && /unhealthy/i.test(value.replace(/\{[^{}]*\}/g, ""))) found.push(path);
    } else if (value && typeof value === "object") for (const [key, child] of Object.entries(value)) walk(child, path ? `${path}.${key}` : key);
  };
  walk(catalogs.de, "");
  expect(found).toEqual([]);
});
