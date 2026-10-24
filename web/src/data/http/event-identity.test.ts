// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original coalescer identity guards tested independently of HTTP abort backstops.

import { CancelledError } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { deferred } from "./activity-reader.test-support";
import { createEventReader } from "./event-reader";

it("new event identity starts distinct full work before the old page is released", async () => {
  let generation = 0; const gate = deferred<string[]>();
  const read = vi.fn().mockReturnValueOnce(gate.promise).mockResolvedValue(["new identity"]);
  const reader = createEventReader<string[]>(read, () => generation);
  const old = reader.load(); const rejected = old.catch((error: unknown) => error);
  generation++;
  const fresh = reader.load();
  expect(fresh).not.toBe(old);
  expect(await fresh).toEqual(["new identity"]);
  gate.resolve(["old identity"]); expect(await rejected).toBeInstanceOf(CancelledError);
});

it.each([false, true])("rejects old event rows after the initial or trailing page (trailing=%s)", async (trailing) => {
  let generation = 0; const gate = deferred<string[]>();
  const read = vi.fn(async () => ["initial"]);
  if (trailing) read.mockResolvedValueOnce(["initial"]);
  read.mockReturnValueOnce(gate.promise);
  const reader = createEventReader(read, () => generation);
  const old = reader.load(); const result = old.then((rows) => ({ rows }), (error: unknown) => ({ error }));
  if (trailing) { reader.changed(); await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(2)); }
  generation++; gate.resolve(["private old rows"]);
  expect(await result).toMatchObject({ error: expect.any(CancelledError) });
});
