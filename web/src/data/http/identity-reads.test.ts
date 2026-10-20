// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original late-response and ignored-abort identity regression tests.

import { expect, it } from "vitest";
import { createIdentityReads } from "./identity-reads";
import { deferred } from "./activity-reader.test-support";

it("rejects an old response even when the fetch implementation ignores abort", async () => {
  const response = deferred<Response>(); let signal: AbortSignal | undefined;
  const reads = createIdentityReads((async (_input, init) => { signal = init?.signal ?? undefined; return response.promise; }) as typeof fetch);
  const old = reads.fetch("/api/events").catch((error: unknown) => error);
  reads.reset(); expect(signal?.aborted).toBe(true);
  response.resolve(new Response("{}"));
  expect(await old).toMatchObject({ message: "CancelledError" });
});

it("guards a previously captured body reader after headers already arrived", async () => {
  const body = deferred<unknown>();
  const response = new Response("{}"); response.json = () => body.promise;
  const reads = createIdentityReads((async () => response) as typeof fetch);
  const headers = await reads.fetch("/api/events");
  const captured = headers.json;
  const old = captured().catch((error: unknown) => error);
  reads.reset(); body.resolve({ private: "old observation" });
  expect(await old).toMatchObject({ message: "CancelledError" });
});
