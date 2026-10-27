import { Activity, AlertTriangle, Package, ScanSearch, Shield } from "lucide-react";
import { useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useActionRuns } from "@/data/live";
import { useScanAction } from "./use-scan-action";
import { ScanProgress } from "./ScanProgress";
import { useSecurity, useStacks } from "@/data/queries";
import { isHostVisible } from "@/domain/hosts";
import { useMotionEnabled } from "@/shared/theme/use-design-tokens";
import { NAV_TAB, NAV_TABS_LIST } from "@/shared/components/nav-tabs";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
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
  const action = useScanAction();
  const scanCapability = useCapability("runScan");
  const enableCapability = useCapability("setScanEnabled");
  const security = useSecurity();
  const stacks = useStacks();
  const hosts = useHostScope();
  const animate = useMotionEnabled();
  const runs = useActionRuns();
  const [params, setParams] = useSearchParams();
  const [highlight, setHighlight] = useState<string | null>(null);
  const tab = (["overview", "findings", "images"].includes(params.get("tab") ?? "") ? params.get("tab") : "overview") as "overview" | "findings" | "images";
  const scan = runs.find((r) => r.kind === "scan" && r.status === "running" && isHostVisible(hosts.selection, r.hostId ?? "0"));

  const data = useMemo(() => {
    const s = security.data;
    if (!s) return null;
    return {
      images: s.images.filter((i) => isHostVisible(hosts.selection, i.hostId)).sort((a, b) => weightOf(b) - weightOf(a)),
      findings: s.findings.filter((f) => isHostVisible(hosts.selection, f.hostId)),
      history: s.history.filter((r) => !r.hostId || isHostVisible(hosts.selection, r.hostId)),
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
              <Button disabled={action.busy || !!scan || !security.data?.enabled || !scanCapability.available} title={scanCapability.reason} onClick={() => void action.start({ hostIds: hosts.selection })} className="group">
                {scan ? <Spinner data-icon="inline-start" /> : <ScanSearch data-icon="inline-start" className="tilt" />}
                {scan ? t("scanning") : t("scan")}
              </Button>
            </HelpTarget>
          </>
        }
      />
      <ScanProgress selection={hosts.selection} />
      {action.error && <p role="alert" className="mb-3">{action.error}</p>}
      {security.data?.unavailable?.filter((h) => isHostVisible(hosts.selection, h.hostId)).map((h) => <Alert key={h.hostId} className="mb-3"><AlertDescription>{t("hostUnavailable", { host: hosts.byId.get(h.hostId)?.name ?? h.hostId })}</AlertDescription></Alert>)}
      {adapter.kind === "http" && <p className="mb-3 text-sm text-muted-foreground">{t("notCovered")}</p>}
      <Alert className="mb-5 border-info/25 bg-info/6">
        <AlertTriangle className="text-info" />
        <AlertDescription className="text-xs">{t("separate")}</AlertDescription>
      </Alert>
      {security.isPending || hosts.isPending ? (
        <SecuritySkeleton />
      ) : security.isError ? (
        <QueryError error={security.error} onRetry={() => void security.refetch()} />
      ) : !security.data.enabled ? (
        <Card className="items-center gap-3 px-6 py-12 text-center">
          <Shield className="size-9 text-muted-foreground" />
          <h2 className="text-lg font-semibold">{t("off.title")}</h2>
          <p className="max-w-md text-sm text-muted-foreground">{t("off.body")}</p>
          <Button disabled={action.busy || !enableCapability.available || !scanCapability.available} title={enableCapability.reason ?? scanCapability.reason} onClick={() => void action.enable({ hostIds: hosts.selection }, true)}>{t("off.enable")}</Button>
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
