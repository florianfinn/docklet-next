// @vitest-environment node
// Source: docklet-next/docs/design/identity.md
// Revision: 5b7a533b0682154acf719fc2ddd059aed9a443c2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original cookie/header interleaving and bounded CSRF recovery tests.

import { expect, it } from "vitest";
import { createCsrfClient } from "./csrf-client";

const base = "https://hub.example.com/api";
const response = (token: string, status = 200, body = "{}") => new Response(body, { status, headers: { "X-CSRF-Token": token } });

it("serializes tokenless GETs when cookie commits and response completion have opposite orders", async () => {
  let cookie = "";
  const pending: { token: string; resolve: (value: Response) => void }[] = [];
  const sent: string[] = [];
  const client = createCsrfClient(base, async (_input, init) => {
    if (init?.method === "POST") {
      const token = new Headers(init.headers).get("X-CSRF-Token");
      expect(token).toBe(cookie);
      return response(cookie);
    }
    const capturedCookie = cookie;
    sent.push(capturedCookie);
    if (capturedCookie) return response(capturedCookie);
    const token = `token-${pending.length + 1}`;
    return new Promise<Response>(resolve => pending.push({ token, resolve }));
  });
  const first = client.fetch(`${base}/auth/mode`);
  const second = client.fetch(`${base}/auth/me`);
  await Promise.resolve();
  // Browser cookies commit at headers; body completion can occur in reverse order.
  for (const item of pending) cookie = item.token;
  for (const item of [...pending].reverse()) {
    item.resolve(response(item.token));
    await Promise.resolve();
  }
  await Promise.all([first, second]);
  await client.fetch(`${base}/auth/login`, { method: "POST", body: "{}" });
  expect(sent).toEqual(["", "token-1"]);
});

it("refreshes on a CSRF-specific 403 and retries a write once with the browser cookie", async () => {
  let cookie = "first";
  const writes: (string | null)[] = [];
  const client = createCsrfClient(base, async (_input, init) => {
    if (init?.method !== "POST") return response(cookie);
    const header = new Headers(init.headers).get("X-CSRF-Token");
    writes.push(header);
    if (header !== cookie) return new Response(JSON.stringify({ code: "forbidden", message: "A valid CSRF token is required" }), { status: 403 });
    return response(cookie);
  });
  await client.fetch(`${base}/auth/mode`);
  cookie = "other-tab";
  expect((await client.fetch(`${base}/settings`, { method: "POST", body: "{}" })).status).toBe(200);
  expect(writes).toEqual(["first", "other-tab"]);
});

it.each(["A valid CSRF token is required", "Permission denied"])("bounds retries for %s", async message => {
  let gets = 0;
  let writes = 0;
  const client = createCsrfClient(base, async (_input, init) => {
    if (init?.method !== "PUT") { gets++; return response("token"); }
    writes++;
    return new Response(JSON.stringify({ code: "forbidden", message }), { status: 403 });
  });
  const result = await client.fetch(`${base}/settings`, { method: "PUT", body: "{}" });
  expect(result.status).toBe(403);
  expect(writes).toBe(message.includes("CSRF") ? 2 : 1);
  expect(gets).toBe(message.includes("CSRF") ? 2 : 1);
});
