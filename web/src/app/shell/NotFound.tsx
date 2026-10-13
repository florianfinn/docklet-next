import { Compass } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { EmptyState } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";

export function NotFound() {
  const t = useTranslations("shell.notFound");
  return (
    <EmptyState
      className="mt-10"
      icon={<Compass />}
      title={t("title")}
      description={t("body")}
      action={
        <Button asChild>
          <Link to="/dashboard">{t("back")}</Link>
        </Button>
      }
    />
  );
}
