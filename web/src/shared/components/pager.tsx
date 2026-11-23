// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original shared accessible fixed-page navigation using free shadcn buttons. R1: focus kept at the ends, optional announcement, hidden for one page.

import { useRef } from "react";
import { useTranslations } from "use-intl";

import { Button } from "@/shared/ui/button";

export interface PagerProps {
  page: number;
  pageCount: number;
  setPage: (page: number) => void;
  /** False where the surrounding range text already announces page changes. */
  announce?: boolean;
}

/** Native buttons retain keyboard access; a button that becomes disabled at an end hands focus to the other one. */
export function Pager({ page, pageCount, setPage, announce = true }: PagerProps) {
  const t = useTranslations("common.pager");
  const previous = useRef<HTMLButtonElement>(null);
  const next = useRef<HTMLButtonElement>(null);
  if (pageCount <= 1) return null;
  return <nav aria-label={t("label")} className="flex flex-wrap items-center justify-end gap-2 p-1 text-xs">
    <Button ref={previous} type="button" variant="outline" size="sm" disabled={page <= 1} onClick={() => { setPage(page - 1); if (page - 1 <= 1) next.current?.focus(); }}>{t("previous")}</Button>
    <span aria-live={announce ? "polite" : undefined} className="tabular-nums">{t("page", { page, pages: pageCount })}</span>
    <Button ref={next} type="button" variant="outline" size="sm" disabled={page >= pageCount} onClick={() => { setPage(page + 1); if (page + 1 >= pageCount) previous.current?.focus(); }}>{t("next")}</Button>
  </nav>;
}
