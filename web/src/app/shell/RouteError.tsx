import { RotateCw, TriangleAlert } from "lucide-react";
import { useRouteError } from "react-router";
import { useTranslations } from "use-intl";

import { EmptyState } from "@/shared/components/page";
import { useDemoText } from "@/shared/hooks/use-demo-text";
import { Button } from "@/shared/ui/button";

/** Route-level fallback, e.g. when a lazy chunk fails to load after a deploy. */
export function RouteError() {
  const t = useTranslations("shell.routeError");
  const dt = useDemoText();
  const error = useRouteError();
  return (
    <EmptyState
      className="mt-10"
      icon={<TriangleAlert />}
      title={t("title")}
      description={error instanceof Error ? dt(error.message) : t("body")}
      action={
        <Button onClick={() => window.location.reload()}>
          <RotateCw data-icon="inline-start" /> {t("reload")}
        </Button>
      }
    />
  );
}
