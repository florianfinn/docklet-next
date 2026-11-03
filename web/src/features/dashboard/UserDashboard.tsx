// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original reduced dashboard for simple users, issue #332 (decision K6).

import { Globe } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { useServiceWidgets } from "@/data/queries";
import { SERVICE_KINDS } from "@/domain/service-widgets";
import { PageHeader, QueryError } from "@/shared/components/page";
import { IconTile } from "@/shared/components/tone";
import { useHostScope, useVisibleStacks } from "@/shared/hooks/use-host-scope";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

import { AppIcon } from "./services/app-icons";
import { ServiceWidgetBody } from "./services/ServiceWidgets";
import { AppsWidget } from "./widgets/SmallWidgets";

function UserCard({ title, icon, children, ...rest }: { title: string; icon: ReactNode; children: ReactNode; "data-user-widget": string }) {
  return (
    <Card className="min-w-0" {...rest}>
      <CardHeader className="flex flex-row items-center gap-3">
        {icon}
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="min-w-0">{children}</CardContent>
    </Card>
  );
}

/** Simple users see configured service widgets and the apps of their assigned stacks, never infrastructure data or Overseerr actions. */
export default function UserDashboard() {
  const t = useTranslations("dashboard");
  const widgets = useServiceWidgets();
  const hosts = useHostScope();
  const stacks = useVisibleStacks();
  const shown = SERVICE_KINDS.flatMap((kind) => {
    const summary = widgets.data?.find((w) => w.kind === kind);
    return summary?.configured && summary.enabled ? [summary] : [];
  });
  return (
    <>
      <PageHeader title={t("title")} description={t("userDescription")} />
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {widgets.isError && <QueryError error={widgets.error} onRetry={() => void widgets.refetch()} />}
        {widgets.isPending && <Skeleton className="h-64 rounded-2xl" />}
        {shown.map((summary) => (
          <UserCard key={summary.kind} data-user-widget={summary.kind} title={t(`services.${summary.kind}.name`)} icon={<AppIcon kind={summary.kind} />}>
            <ServiceWidgetBody kind={summary.kind} summary={summary} admin={false} />
          </UserCard>
        ))}
        <UserCard data-user-widget="apps" title={t("widget.apps.title")} icon={<IconTile tone="sky" size="sm"><Globe /></IconTile>}>
          {stacks.isPending ? <Skeleton className="h-24 rounded-xl" /> : <AppsWidget stacks={stacks.visible} hosts={hosts.byId} />}
        </UserCard>
      </div>
    </>
  );
}
