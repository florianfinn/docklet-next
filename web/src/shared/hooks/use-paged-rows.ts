// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original synchronous bounded paging with target and identity anchoring. R1: paging by hand stops following a moving target.

import { useState } from "react";

interface PageOptions {
  targetKey?: string;
  resetKey?: string;
}

/**
 * Keeps a bounded slice even on the render that receives a new target or identity. A new reset key, target or size
 * anchors on the target's page and follows the target while data moves; paging by hand ends following until the next anchor.
 */
export function usePagedRows<T extends { key: string }>(rows: readonly T[], pageSize: number, { targetKey, resetKey }: PageOptions = {}) {
  const size = Math.max(1, Math.floor(pageSize) || 1);
  const pageCount = Math.max(1, Math.ceil(rows.length / size));
  const targetIndex = targetKey === undefined ? -1 : rows.findIndex((row) => row.key === targetKey);
  const targetPage = targetIndex < 0 ? undefined : Math.floor(targetIndex / size) + 1;
  const anchor = JSON.stringify([resetKey, targetKey, size]);
  const [state, setState] = useState(() => ({ anchor, page: targetPage ?? 1, follow: true }));
  const fresh = state.anchor !== anchor;
  const follow = fresh || state.follow;
  const requested = follow && targetPage !== undefined ? targetPage : fresh ? 1 : state.page;
  const page = Math.max(1, Math.min(pageCount, requested));
  if (fresh || state.page !== page || state.follow !== follow) setState({ anchor, page, follow });
  const start = (page - 1) * size;
  return {
    page,
    pageCount,
    visible: rows.slice(start, start + size),
    from: rows.length ? start + 1 : 0,
    to: Math.min(start + size, rows.length),
    setPage: (next: number) => setState({ anchor, page: Math.max(1, Math.min(pageCount, Math.floor(next) || 1)), follow: false }),
    targetHidden: targetKey !== undefined && targetIndex < 0,
  };
}
