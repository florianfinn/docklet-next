import { AlertTriangle, RotateCw } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/shared/ui/empty";

/** `compact` (resource lists): on phones the intro is hidden and the actions stay beside the title. */
export function PageHeader({ title, description, actions, eyebrow, compact }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; eyebrow?: ReactNode; compact?: boolean }) {
  return (
    <header className={cn("mb-6 flex flex-wrap items-end gap-3", compact && "max-md:mb-3 max-md:items-center")}>
      {/* The title keeps a readable width; actions wrap below it on narrow screens. */}
      <div className={cn("min-w-[min(100%,20rem)] flex-1", compact && "max-md:min-w-0")}>
        {eyebrow}
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && <p className={cn("mt-0.5 max-w-4xl text-sm text-muted-foreground", compact && "max-md:hidden")}>{description}</p>}
      </div>
      {actions && (
        <div data-page-actions className="flex flex-wrap items-center gap-2">
          {actions}
        </div>
      )}
    </header>
  );
}

export function QueryError({ onRetry, className, compact, error, message }: { onRetry?: () => void; className?: string; compact?: boolean; error?: unknown; message?: string }) {
  const t = useTranslations("common");
  const capability = useTranslations("activity.capability");
  // A read refused by hub maintenance is not a failure of the source; the shell shows the maintenance status.
  if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "backup.maintenance") {
    return <div role="status" className={cn("text-sm text-muted-foreground", className)}><p>{t("query.maintenance")}</p></div>;
  }
  if (error instanceof AdapterOutcomeError && error.outcome.kind === "not-available") {
    return <div role="status" className={cn("space-y-2 text-sm text-muted-foreground", className)}><p>{t("query.unavailableTitle")}</p><p>{capability(error.outcome.reasonCode === "capability.mapping-unavailable" ? "mappingUnavailable" : "unavailable")}</p></div>;
  }
  return (
    <Alert variant="destructive" className={cn("border-destructive/30 bg-destructive/5", className)}>
      <AlertTriangle />
      <AlertTitle>{message ?? t("query.errorTitle")}</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-3">
        {!compact && <span>{t("query.errorHint")}</span>}
        {onRetry && (
          <Button size="sm" variant="outline" onClick={onRetry} className="group">
            <RotateCw data-icon="inline-start" className="spin-on-hover" />
            {t("actions.retry")}
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}

export function EmptyState({ icon, title, description, action, className }: { icon: ReactNode; title: string; description?: string; action?: ReactNode; className?: string }) {
  return (
    <Empty className={cn("border border-dashed bg-card/40", className)}>
      <EmptyHeader>
        <EmptyMedia variant="icon">{icon}</EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}
