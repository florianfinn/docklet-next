import { Activity, AlertTriangle, Package, ScanSearch, Shield } from "lucide-react";
import { useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { useActionRuns } from "@/data/live";
import { useSecurity, useStacks } from "@/data/queries";
import { isHostVisible } from "@/domain/hosts";
import { withPercent } from "@/domain/locale";
import { useMotionEnabled } from "@/shared/theme/use-design-tokens";
import { NAV_TAB, NAV_TABS_LIST } from "@/shared/components/nav-tabs";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Progress } from "@/shared/ui/progress";
import { Skeleton } from "@/shared/ui/skeleton";
import { Spinner } from "@/shared/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";

import { ScanScheduleLink, ScanToggle } from "./ScanControls";
import { weightOf } from "./severity";
import { FindingsTab, ImagesTab, OverviewTab } from "./SecurityTabs";

const TABS = [
  { id: "overview", icon: Activity },
  { id: "findings", icon: AlertTriangle },
  { id: "images", icon: Package },
] as const;

/** Tabs take the navigation look (nav-row): primary tint on hover, primary surface, edge and bar when active. */

function SecuritySkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-6">
      <Skeleton className="h-9 w-80 rounded-lg" />
      <div className="grid gap-8 lg:grid-cols-[1fr_1.5fr]">
        <Skeleton className="mx-auto h-[190px] w-full max-w-80 rounded-t-full" />
        <div className="flex flex-col gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-44 rounded-xl" />
          <Skeleton className="h-10 rounded-lg" />
        </div>
      </div>
      <Skeleton className="h-24 rounded-xl" />
    </div>
  );
}

export default function SecurityPage() {
  const t = useTranslations("security");
  const adapter = useAdapter();
  const security = useSecurity();
  const stacks = useStacks();
  const hosts = useHostScope();
  const animate = useMotionEnabled();
  const runs = useActionRuns();
  const [params, setParams] = useSearchParams();
  const [highlight, setHighlight] = useState<string | null>(null);
  const tab = (["overview", "findings", "images"].includes(params.get("tab") ?? "") ? params.get("tab") : "overview") as "overview" | "findings" | "images";
  const scan = runs.find((r) => r.kind === "scan" && r.status === "running");

  const data = useMemo(() => {
    const s = security.data;
    if (!s) return null;
    return {
      images: s.images.filter((i) => isHostVisible(hosts.selection, i.hostId)).sort((a, b) => weightOf(b) - weightOf(a)),
      findings: s.findings.filter((f) => isHostVisible(hosts.selection, f.hostId)),
      history: s.history,
    };
  }, [security.data, hosts.selection]);

  const setTab = (v: string) => setParams((p) => ({ ...Object.fromEntries(p), tab: v }), { replace: true });

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <>
            {security.data && <ScanToggle enabled={security.data.enabled} />}
            {security.data?.enabled && <ScanScheduleLink schedule={security.data.schedule} enabled />}
            <HelpTarget topic="action.scan" side="bottom">
              <Button disabled={!!scan} onClick={() => void adapter.runScan()} className="group">
                {scan ? <Spinner data-icon="inline-start" /> : <ScanSearch data-icon="inline-start" className="tilt" />}
                {scan ? t("scanning") : t("scan")}
              </Button>
            </HelpTarget>
          </>
        }
      />
      {scan && (
        <div className="mb-4 flex items-center gap-3 rounded-xl border border-primary/30 bg-primary/8 px-4 py-2.5 text-sm">
          <Activity className="size-4 text-primary-text" />
          <span className="shrink-0">{scan.steps.find((s) => s.state === "running")?.label ?? t("scanning")}</span>
          <Progress value={scan.progress} className="h-1.5 flex-1" />
          <span className="text-xs text-muted-foreground tabular">{withPercent(scan.progress)}</span>
        </div>
      )}
      <Alert className="mb-5 border-info/25 bg-info/6">
        <AlertTriangle className="text-info" />
        <AlertDescription className="text-xs">{t("separate")}</AlertDescription>
      </Alert>
      {security.isPending || hosts.isPending ? (
        <SecuritySkeleton />
      ) : security.isError ? (
        <QueryError onRetry={() => void security.refetch()} />
      ) : !security.data.enabled ? (
        <Card className="items-center gap-3 px-6 py-12 text-center">
          <Shield className="size-9 text-muted-foreground" />
          <h2 className="text-lg font-semibold">{t("off.title")}</h2>
          <p className="max-w-md text-sm text-muted-foreground">{t("off.body")}</p>
          <Button onClick={() => void adapter.runScan()}>{t("off.enable")}</Button>
        </Card>
      ) : !data?.images.length ? (
        <EmptyState icon={<Shield />} title={t("empty.title")} description={t("empty.body")} />
      ) : (
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList aria-label={t("tabsLabel")} className={cn("mb-4", NAV_TABS_LIST)}>
            {TABS.map(({ id, icon: Icon }) => (
              <TabsTrigger key={id} value={id} data-active={tab === id} className={NAV_TAB}>
                <Icon aria-hidden /> {t(`tabs.${id}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="overview">
            <OverviewTab
              {...data}
              stacks={stacks.data ?? []}
              hosts={hosts.byId}
              animate={animate}
              onFocus={(id) => {
                setHighlight(id);
                setTab("findings");
              }}
            />
          </TabsContent>
          <TabsContent value="findings">
            <FindingsTab findings={data.findings} hosts={hosts.byId} highlight={highlight} />
          </TabsContent>
          <TabsContent value="images">
            <ImagesTab images={data.images} stacks={stacks.data ?? []} hosts={hosts.byId} />
          </TabsContent>
        </Tabs>
      )}
    </>
  );
}
