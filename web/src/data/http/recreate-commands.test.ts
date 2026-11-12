// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W3 generated edit, retry header and retained-result transport proof.

import { expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import { envelope, response, RUN_ID, runtimeFixture, snapshot } from "./runtime-commands.test-support";

it.each(["0", "direct", "edge"])("W3 %s sends the generated recreate request with a stable retry key and retains replacement identity", async (host) => {
  const f = runtimeFixture();
  f.state.run = snapshot(host, { type: "container_edit", resourceId: "old", status: "success", outcomeState: "known", result: { oldId: "old", newId: "replacement", oldName: "app", newName: "renamed" } });
  const edit: components["schemas"]["ContainerRecreateRequest"]["edit"] = { image: "example/app:2", name: "renamed", environment: ["EXAMPLE=value"], hostConfig: { restartPolicy: { name: "always" }, portBindings: { "80/tcp": [{ hostPort: "8080", hostIp: "192.0.2.10" }] } } };
  let attempts = 0;
  f.state.command = async () => { if (++attempts === 1) throw new TypeError("lost response"); return envelope({ activityId: RUN_ID, code: "run.joined", activity: f.state.run }); };
  const key = "9695d273-8b96-4752-9c7c-d6edfc51c670";
  expect(await f.adapter.recreateContainer(`${host}~old`, edit, key)).toBe(RUN_ID);
  const calls = f.commands();
  expect(calls).toHaveLength(2);
  for (const call of calls) {
    expect(call.url.pathname).toBe(`/api/environments/${host}/containers/old/recreate`);
    expect(call.init.method).toBe("POST");
    expect(JSON.parse(call.init.body as string)).toEqual({ edit });
    expect(new Headers(call.init.headers).get("Idempotency-Key")).toBe(key);
    expect(new Headers(call.init.headers).get("Prefer")).toBe("respond-async");
  }
  expect(await f.adapter.getRuntimeRun!(host, RUN_ID)).toMatchObject({ recreateResult: f.state.run.result });
});

it("W3 maps the actual idempotency mismatch and recovered/ambiguous problems", async () => {
  for (const code of ["runtime.idempotency-mismatch", "container.recreate-recovered", "container.recreate-ambiguous"] as const) {
    const f = runtimeFixture();
    const problem: components["schemas"]["ContainerProblem"] = { code, status: 409 };
    f.state.command = async () => response(problem, 409);
    await expect(f.adapter.recreateContainer("0~old", { image: "example/app:2" }, "example")).rejects.toMatchObject({ outcome: { code, status: 409 } });
  }
});
