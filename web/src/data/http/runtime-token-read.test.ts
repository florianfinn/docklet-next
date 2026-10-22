// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original token error identity and pre-admission classification regressions for issue #128.

import { expect, it } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { runtimeTokenReads } from "./runtime-token-read";
import { envelope, runtimeFixture } from "./runtime-commands.test-support";

it.each(["completeSetup", "settings"])("%s preserves the original token transport error object and diagnostics", async (consumer) => {
  const original = new TypeError("Example token transport unavailable");
  const adapter = createHttpAdapter({ fetch: async () => { throw original; } });
  const general = { hubName: "Example", hubAddress: "", domainUrl: "", linkBase: "auto" as const, timeZone: "UTC" };
  const pending = consumer === "settings" ? adapter.saveSettings("general", general) : adapter.completeSetup({ general,
    auth: { mode: "none" }, hostDefaults: { connection: "direct", projectDir: "/srv/projects", installDir: "/srv/agent", puid: "1000", pgid: "1000", detectSocketGroup: true } } as Parameters<typeof adapter.completeSetup>[0]);
  const error = await pending.catch((error: unknown) => error);
  expect(error === original).toBe(true);
  expect(error).toBeInstanceOf(TypeError);
  expect((error as Error).message).toBe(original.message);
});
it("token tagging preserves frozen errors without modifying them", async () => {
  const original = Object.freeze(new TypeError("Example frozen failure"));
  const fetcher = runtimeTokenReads("/api", async () => { throw original; });
  expect(await fetcher("/api/auth/mode").catch((error: unknown) => error) === original).toBe(true);
});
it("a refresh transport failure after an earlier possibly admitted POST stays ambiguous", async () => {
  const f = runtimeFixture(); let reads = 0;
  f.state.auth = () => { if (++reads === 1) return envelope({}); throw new TypeError("Example token refresh loss"); };
  f.state.command = () => { if (f.commands().length === 1) throw new TypeError("Example admission response loss"); return new Response(JSON.stringify({ code: "csrf.invalid" }), { status: 403 }); };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.commands()).toHaveLength(2);
});

it("an error object reused by a later POST cannot retain bootstrap classification", async () => {
  const f = runtimeFixture(); const original = new TypeError("Example shared transport error");
  f.state.auth = () => { throw original; };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.pre-admission" } });
  expect(f.commands()).toHaveLength(0);
  f.state.auth = undefined;
  f.state.command = () => { throw original; };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.commands()).toHaveLength(2);
});
