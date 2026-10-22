// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original status-only response ownership regressions for issue #133.

import { expect, it } from "vitest";
import { createIdentityReads } from "./identity-reads";
import { createCsrfClient } from "./csrf-client";
import { createHttpAuth } from "./auth";
import { createSettingsCore } from "./settings-core";
import { createHostCommands } from "./host-commands";
import { csrf } from "./runtime-commands.test-support";

it.each(["settings-success", "settings-failure", "setup", "auth-failure", "host-failure"])("%s discards 50 status-only bodies without retaining controllers", async (consumer) => {
  let cancelled = 0, aborted = 0;
  const reads = createIdentityReads(async (input, init) => {
    if (String(input).endsWith("/auth/mode")) return new Response("{}", { headers: { "X-CSRF-Token": csrf } });
    init?.signal?.addEventListener("abort", () => { aborted++; });
    return new Response(new ReadableStream({ cancel() { cancelled++; } }), { status: consumer.includes("failure") ? 503 : 200 });
  });
  const client = createCsrfClient("/api", reads.fetch);
  const settings = createSettingsCore("/api", client);
  const auth = createHttpAuth("/api", client);
  const hosts = createHostCommands("/api", client, async () => {}, () => {});
  for (let i = 0; i < 50; i++) {
    const general = { hubName: "Example", hubAddress: "", domainUrl: "", linkBase: "auto" as const, timeZone: "UTC" };
    const action = consumer === "auth-failure" ? auth.getCurrentUser() : consumer === "host-failure" ? hosts.removeHost("example")
      : consumer === "setup" ? settings.completeSetup({ general, auth: { mode: "none" }, hostDefaults: { connection: "direct", projectDir: "/srv/projects", installDir: "/srv/agent", puid: "1000", pgid: "1000", detectSocketGroup: true } } as Parameters<typeof settings.completeSetup>[0])
      : settings.saveSettings("general", general);
    await action.catch(() => undefined);
  }
  expect(cancelled).toBe(50);
  reads.reset();
  expect(aborted).toBe(0);
});
