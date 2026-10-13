import { Plus, Radio, Server, ServerCog, Unplug } from "lucide-react";
import { lazy, Suspense, useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useContainers, useStacks, useSystemComponents } from "@/data/queries";
import type { Host } from "@/domain/model";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";

import { HostCard } from "./HostCard";
const InstallSheet = import.meta.env.DEV ? lazy(() => import("./InstallSheet.dev")) : null;

export default function HostsPage() {
  const t = useTranslations("hosts");
  const hosts = useHostScope();
  const stacks = useStacks();
  const containers = useContainers();
  const system = useSystemComponents();
  const [filesFor, setFilesFor] = useState<Host | null>(null);
  const ids = hosts.all.map((h) => h.id);
  const count = (hostId: string) => ({
    stacks: (stacks.data ?? []).filter((s) => s.hostId === hostId).length,
    containers: (containers.data ?? []).filter((c) => c.hostId === hostId).length,
  });

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button asChild className="group">
            <Link to="/hosts/new">
              <Plus data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:rotate-90" />
              {t("add")}
            </Link>
          </Button>
        }
      />
      <StatStrip
        items={[
          { label: t("strip.total"), value: hosts.isPending ? "–" : hosts.all.length, icon: <Server /> },
          { label: t("strip.online"), value: hosts.isPending ? "–" : hosts.all.filter((h) => h.status === "online").length, icon: <ServerCog />, tone: "ok" },
          { label: t("strip.edge"), value: hosts.isPending ? "–" : hosts.all.filter((h) => h.connection === "edge").length, icon: <Radio />, tone: "info" },
          { label: t("strip.attention"), value: hosts.isPending ? "–" : hosts.all.filter((h) => h.status !== "online").length, icon: <Unplug />, tone: hosts.all.some((h) => h.status === "offline") ? "err" : hosts.all.some((h) => h.status === "pending") ? "info" : undefined },
        ]}
      />
      {hosts.isPending ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,34rem),1fr))] gap-4">
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-[23rem] rounded-2xl" />
          ))}
        </div>
      ) : hosts.isError ? (
        <QueryError onRetry={() => void hosts.refetch()} />
      ) : hosts.all.length === 0 ? (
        <EmptyState
          icon={<Server />}
          title={t("title")}
          action={
            <Button asChild>
              <Link to="/hosts/new">{t("add")}</Link>
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,34rem),1fr))] gap-4">
          {hosts.all.map((h) => (
            <HostCard key={h.id} host={h} system={system.data?.find((c) => c.hostId === h.id)} {...count(h.id)} allIds={ids} onFiles={import.meta.env.DEV ? setFilesFor : undefined} />
          ))}
        </div>
      )}
      {InstallSheet && filesFor && <Suspense fallback={null}><InstallSheet host={filesFor} onClose={() => setFilesFor(null)} /></Suspense>}
    </>
  );
}
