// Source: docklet-next/issues/389
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original availability policy for expendable background host reads.

/** Tag set by readers on expendable automatic requests; the dispatch gate removes it before the network. */
export const READ_TAG_HEADER = "X-Docklet-Read";
/** `background`: fan-out on mount, invalidation or recovery; `poll`: an interval refresh. */
export type ReadTag = "background" | "poll";
/** Marks an answer the gate produced without a request; host admission does not treat it as a held host. */
export const SKIPPED_HEADER = "X-Docklet-Skipped";

type Row = { id?: unknown; status?: unknown; enabled?: unknown };
const SKIP = new Set(["offline", "error", "pending", "update-required", "token-rejected"]);

/** The visible refusal of a host whose reads are skipped; statuses with the same refusal read alike. */
export function skipReason(status: string | undefined): string {
  if (status === "update-required") return "agent.update-required";
  if (status === "token-rejected") return "agent.invalid-token";
  return "agent.unavailable";
}

/** A catalogue status whose automatic reads are skipped. */
export const skipsReads = (status: string | undefined) => !!status && SKIP.has(status);

/** Current catalogue status per host. Environment stream rows are authoritative; a catalogue list answer only counts
 * when its read started after the last applied stream envelope, so a delayed older list never undoes a newer state. */
export function createHostReadPolicy() {
  const statuses = new Map<string, string>();
  let sequence = 0;
  function apply(rows: readonly Row[]) {
    statuses.clear();
    for (const row of rows) {
      if (typeof row?.id !== "string" || typeof row.status !== "string") continue;
      statuses.set(row.id, row.enabled === false ? "offline" : row.status);
    }
  }
  return {
    /** Stream rows, applied before the stream's invalidations and listeners run. */
    applyStream(rows: readonly Row[]) { ++sequence; apply(rows); },
    /** Start of a catalogue list read; pass the token to `applyCatalogue` with its answer. */
    beginCatalogue: () => sequence,
    applyCatalogue(token: number, rows: readonly Row[]) { if (token === sequence) apply(rows); },
    /** Unknown hosts are read; standby skips interval polls only, so a mount still wakes the tunnel. */
    skip(hostId: string, tag: ReadTag): boolean {
      const status = statuses.get(hostId);
      if (status === undefined) return false;
      return SKIP.has(status) || status === "standby" && tag === "poll";
    },
    reset() { ++sequence; statuses.clear(); },
  };
}

export type HostReadPolicy = ReturnType<typeof createHostReadPolicy>;
