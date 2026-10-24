import { RotateCw, TriangleAlert } from "lucide-react";
import { useTranslations } from "use-intl";

import { EmptyState } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";

/** Route-level fallback, e.g. when a lazy chunk fails to load after a deploy. */
export function RouteError() {
  const t = useTranslations("shell.routeError");
  return (
    <EmptyState
      className="mt-10"
      icon={<TriangleAlert />}
      title={t("title")}
      description={t("body")}
      action={
        <Button onClick={() => window.location.reload()}>
          <RotateCw data-icon="inline-start" /> {t("reload")}
        </Button>
      }
    />
  );
}
