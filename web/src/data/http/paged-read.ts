// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Moved verbatim from http-adapter.ts (M23 size limit): sequential page reads of hub lists.

import { CancelledError } from "@tanstack/react-query";
import type { operations, paths } from "@/api/generated/schema";

export type ReadOperation = "get-capabilities" | "listEnvironments" | "list-activities" | "get-activity" | "listEvents";
export type Body<I extends ReadOperation> = operations[I]["responses"][200]["content"]["application/json"];
type PagedOperation = "listEnvironments" | "list-activities" | "listEvents";
type Read = <I extends ReadOperation>(operationId: I, route: keyof paths | string, query?: Record<string, string | number>, observeResponse?: (response: Response) => void, signal?: AbortSignal) => Promise<Body<I>>;

/** Reads every page of a hub list; `limit = -1` reads one complete page. */
export function createPager(read: Read, classifiedError: (error: unknown) => Error) {
  return async function page<I extends PagedOperation>(operationId: I, route: string, limit: number, observeResponse?: (response: Response) => void, signal?: AbortSignal): Promise<NonNullable<Body<I>["data"]>> {
    const result: NonNullable<Body<I>["data"]> = [] as unknown as NonNullable<Body<I>["data"]>;
    let start = 0;
    for (;;) {
      if (signal?.aborted) throw new CancelledError();
      let responseIncomplete = false;
      const body = await read(operationId, route, { start, limit, order: "desc", ...(operationId === "list-activities" ? { includeSummary: "true" } : {}), ...(operationId === "listEvents" ? { sort: "timestamp" } : {}) }, (response) => { responseIncomplete = response.headers.get("X-Docklet-Host-Reason") === "agent.unavailable"; observeResponse?.(response); }, signal);
      if (!Array.isArray(body.data) || !body.pagination || !Number.isInteger(body.pagination.totalItems)) throw classifiedError({ code: `${operationId}.invalid-page` });
      result.push(...body.data as never[]);
      start += body.data.length;
      if (limit === -1 && start < body.pagination.totalItems && responseIncomplete === false) throw classifiedError({ code: `${operationId}.short-page` });
      if (limit === -1 || start >= body.pagination.totalItems) return result;
      if (body.data.length === 0) throw classifiedError({ code: `${operationId}.short-page` });
    }
  };
}
