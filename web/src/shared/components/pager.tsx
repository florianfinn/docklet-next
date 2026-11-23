// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original shared accessible fixed-page navigation using free shadcn buttons.

import { useTranslations } from "use-intl";

import { Button } from "@/shared/ui/button";

export interface PagerProps {
  page: number;
  pageCount: number;
  setPage: (page: number) => void;
}

/** Native buttons retain keyboard access and focus across page changes. */
export function Pager({ page, pageCount, setPage }: PagerProps) {
  const t = useTranslations("common.pager");
  return <nav aria-label={t("label")} className="flex flex-wrap items-center justify-end gap-2 p-1 text-xs">
    <Button type="button" variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>{t("previous")}</Button>
    <span aria-live="polite" className="tabular-nums">{t("page", { page, pages: pageCount })}</span>
    <Button type="button" variant="outline" size="sm" disabled={page >= pageCount} onClick={() => setPage(page + 1)}>{t("next")}</Button>
  </nav>;
}
