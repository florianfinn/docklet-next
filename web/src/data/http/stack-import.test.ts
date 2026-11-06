// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original recorded contract response and request mapping proofs. M19 web consumer integration.

import { expect, it, vi } from "vitest";
import { createStackImport } from "./stack-import";
const result = { title: "example-app", composeContent: "services:\n  app:\n    image: example/app:1\n", envNames: ["TOKEN"], found: ["import.found.compose"], missing: ["import.missing.variables"], source: { kind: "github", canonicalRef: "example/app" } };
const grant = async (operationId: string, resourceRef = { kind: "hub" as const, id: "0" }) => ({ operationId, resourceRef, available: true });
it("maps the raw frozen producer response and Docker Hub kind using CSRF client", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify(result)));
  const api = createStackImport("/api", { fetch: fetcher, token: async () => "" }, () => 0, grant);
  expect(await api.importSource({ kind: "dockerhub", ref: "example/app:1" })).toEqual({ ...result, source: undefined });
  expect(fetcher.mock.calls[0]).toMatchObject(["/api/stack-import", { method: "POST", body: JSON.stringify({ kind: "registry", ref: "example/app:1" }) }]);
});
it("keeps typed problems and discards results from the previous identity", async () => {
  let generation = 0;
  const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ code: "import.timeout", detail: "untrusted upstream text" }), { status: 504 }));
  const api = createStackImport("/api", { fetch: fetcher, token: async () => "" }, () => generation, grant);
  await expect(api.importSource({ kind: "github", ref: "example/app" })).rejects.toMatchObject({ outcome: { code: "import.timeout", status: 504 } });
  fetcher.mockImplementation(async () => { generation++; return new Response(JSON.stringify(result)); });
  await expect(api.importSource({ kind: "github", ref: "example/app" })).rejects.toMatchObject({ revert: undefined });
});
