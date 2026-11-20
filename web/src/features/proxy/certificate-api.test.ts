// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

import { afterEach, expect, it, vi } from "vitest";
import { checkCertificate } from "./certificate-api";
import { hash } from "./proxy.test-support";

const input = { file: "routes/app.yml", router: "app", sourceHash: hash, host: "app.example.test" };
const observation = {
  observedAt: "2026-11-20T12:00:00Z", instanceId: "instance",
  epoch: { containerId: "example-container", startedAt: "2026-11-20T00:00:00Z" },
  route: { name: "app", host: input.host, sni: input.host, entryPoint: "websecure" },
  peer: { subject: "CN=app.example.test", issuer: "CN=Example CA", sans: [], notBefore: "2026-01-01T00:00:00Z", notAfter: "2027-01-01T00:00:00Z", sha256: hash },
  chainTrusted: false, hostnameMatch: true, validity: "valid",
};
afterEach(() => vi.unstubAllGlobals());
it("posts the fenced route through CSRF and decodes the real observation contract", async () => {
  const fetcher = vi.fn(async (path: string, init?: RequestInit) => {
    if (path === "/api/auth/mode") return new Response("{}", { headers: { "X-CSRF-Token": "example" } });
    expect(path).toBe("/api/modules/proxy/instances/instance/certificate-check");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(input);
    return Response.json({ success: true, data: observation });
  });
  vi.stubGlobal("fetch", fetcher);
  expect(await checkCertificate("instance", input, new AbortController().signal)).toEqual(observation);
  expect(fetcher).toHaveBeenCalledTimes(2);
});
it("keeps the typed helper-unavailable refusal", async () => {
  vi.stubGlobal("fetch", async (path: string) => path === "/api/auth/mode" ? new Response("{}", { headers: { "X-CSRF-Token": "example" } }) : Response.json({ code: "certificate-unavailable" }, { status: 409 }));
  await expect(checkCertificate("instance", input, new AbortController().signal)).rejects.toMatchObject({ code: "certificate-unavailable", status: 409 });
});
it("discards a completed response after the owning query was cancelled", async () => {
  const controller = new AbortController();
  vi.stubGlobal("fetch", async (path: string) => {
    if (path === "/api/auth/mode") return new Response("{}", { headers: { "X-CSRF-Token": "example" } });
    controller.abort();
    return Response.json({ success: true, data: observation });
  });
  await expect(checkCertificate("instance", input, controller.signal)).rejects.toThrow();
});
