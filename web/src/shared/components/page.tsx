import { AlertTriangle, RotateCw } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/shared/ui/empty";

export function PageHeader({ title, description, actions, eyebrow }: { title: ReactNode; description?: ReactNode; actions?: ReactNode; eyebrow?: ReactNode }) {
  return (
    <header className="mb-6 flex flex-wrap items-end gap-3">
      {/* The title keeps a readable width; actions wrap below it on narrow screens. */}
      <div className="min-w-[min(100%,20rem)] flex-1">
        {eyebrow}
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-0.5 max-w-4xl text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && (
        <div data-page-actions className="flex flex-wrap items-center gap-2">
          {actions}
        </div>
      )}
    </header>
  );
}

export function QueryError({ onRetry, className, compact }: { onRetry?: () => void; className?: string; compact?: boolean }) {
  const t = useTranslations("common");
  return (
    <Alert variant="destructive" className={cn("border-destructive/30 bg-destructive/5", className)}>
      <AlertTriangle />
      <AlertTitle>{t("query.errorTitle")}</AlertTitle>
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
