// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original synchronous bounded paging with target and identity anchoring.

import { useState } from "react";

interface PageOptions {
  targetKey?: string;
  resetKey?: string;
}

/** Keeps a bounded slice even on the render that receives a new target or identity. */
export function usePagedRows<T extends { key: string }>(rows: readonly T[], pageSize: number, { targetKey, resetKey }: PageOptions = {}) {
  const size = Math.max(1, Math.floor(pageSize) || 1);
  const pageCount = Math.max(1, Math.ceil(rows.length / size));
  const targetIndex = targetKey === undefined ? -1 : rows.findIndex((row) => row.key === targetKey);
  const anchor = JSON.stringify([resetKey, targetKey, targetIndex, size]);
  const [state, setState] = useState(() => ({ anchor, page: targetIndex < 0 ? 1 : Math.floor(targetIndex / size) + 1 }));
  const requested = state.anchor === anchor ? state.page : targetIndex < 0 ? 1 : Math.floor(targetIndex / size) + 1;
  const page = Math.max(1, Math.min(pageCount, requested));
  if (state.anchor !== anchor || state.page !== page) setState({ anchor, page });
  const start = (page - 1) * size;
  return {
    page,
    pageCount,
    visible: rows.slice(start, start + size),
    from: rows.length ? start + 1 : 0,
    to: Math.min(start + size, rows.length),
    setPage: (next: number) => setState({ anchor, page: Math.max(1, Math.min(pageCount, Math.floor(next) || 1)) }),
    targetHidden: targetKey !== undefined && targetIndex < 0,
  };
}
