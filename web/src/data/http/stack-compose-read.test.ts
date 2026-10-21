// Source: docklet-next/docs/design/stack-authoring.md
// Revision: b0f06b6a169842c03597bbb9a7c177ef75d4eea1
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original regression for the existing-stack editor content found in M7 practical acceptance (#137).

import { expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";

it("loads compose.yaml and .env of an existing stack from the compose read", async () => {
  const f = resourceFixture();
  const stack = await f.adapter.getStack("first~project");
  expect(stack.compose).toBe("services:\n  web:\n    image: example:1.0.0\n");
  expect(stack.env).toBe("TZ=UTC\n");
  const paths = f.fetcher.mock.calls.map(([url]) => new URL(String(url), "https://hub.example.com").pathname);
  expect(paths.includes("/api/environments/first/projects/project/compose")).toBe(true);
});

it("keeps the retained stack detail but marks files unavailable when the compose read fails", async () => {
  const f = resourceFixture();
  f.state.filesDown = true;
  f.state.reason = "agent.unavailable";
  const stack = await f.adapter.getStack("first~project");
  expect(stack.filesUnavailable).toBe(true);
  expect(stack.compose).toBe("");
  expect(stack.inventory?.stale).toBe(true);
});
