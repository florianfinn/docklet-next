// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original two-host shell HTTP fixture with independently failing count and container legs.

import { vi } from "vitest";
import { createHttpAdapter } from "@/data/http/http-adapter";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";
import { deferred } from "@/data/http/activity-reader.test-support";

export function navFixture() {
  const f = securityFixture();
  const state = { count: { first: 2, second: 3 } as Record<string, number>, bindSources: ["/srv/example:data", "/srv/example:data"],
    inventoryFailures: new Set<string>(), countFailures: new Set<string>(), extras: false,
    delayHost: "", delay: undefined as ReturnType<typeof deferred<Response>> | undefined, authenticated: true };
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname, host = path.split("/")[3]!;
    if (path.endsWith("/auth/logout")) { state.authenticated = false; return Response.json({ success: true, data: {} }); }
    if (path.endsWith("/auth/login")) { state.authenticated = true; return Response.json({ success: true, data: { user: { id: "operator", username: "operator", isAdmin: true, requiresPasswordChange: false } } }); }
    if (path.endsWith("/auth/me") && !state.authenticated) return Response.json({ code: "auth.unauthorized" }, { status: 401 });
    if (path.endsWith("/volumes/_collection/counts")) {
      if (state.delayHost === host && state.delay) return state.delay.promise;
      return Response.json(state.countFailures.has(host) ? { code: "http.error" } : { success: true, data: { total: state.count[host], inuse: 0, unused: state.count[host] } }, { status: state.countFailures.has(host) ? 503 : 200 });
    }
    if (path.endsWith("/containers")) {
      if (state.inventoryFailures.has(host)) return Response.json({ code: "http.error" }, { status: 503 });
      const response = await f.fetcher(input, init), body = await response.json();
      body.data[0].mounts = state.bindSources.map((source) => ({ type: "bind", source, destination: "/data", rw: true }));
      if (state.extras) body.data.push(
        { ...body.data[0], id: "system", imageId: "sha256:system", labels: { "io.docklet.system": "agent" }, mounts: [{ type: "bind", source: "/srv/system", destination: "/config" }] },
        { ...body.data[0], id: "stopped", imageId: "sha256:stopped", state: "exited", mounts: [] });
      return Response.json(body);
    }
    const response = await f.fetcher(input, init);
    if (state.extras && path.endsWith("/active-summaries") && response.ok) {
      const body = await response.json(), example = body.data.images[0];
      body.data.images.push({ ...example, imageId: "sha256:system" }, { ...example, imageId: "sha256:stopped" });
      return Response.json(body);
    }
    return response;
  });
  return { ...f, state, security: f.state, fetcher, adapter: createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => f.source }) };
}
