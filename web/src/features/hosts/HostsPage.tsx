import { Plus, Radio, Server, ServerCog, Unplug } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useContainers, useSettings, useStacks, useSystemComponents } from "@/data/queries";
import { agentHubUrl } from "@/domain/settings";
import type { Host } from "@/domain/model";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Skeleton } from "@/shared/ui/skeleton";

import { HostCard } from "./HostCard";
import { InstallArtifacts } from "./InstallArtifacts";
import { planFromDraft, type HostDraft } from "./onboarding/onboarding-draft";
import { useHostCommands } from "./use-host-commands";

function draftOf(host: Host, token: string): HostDraft {
  const [address = "", port = "3552"] = host.address.split(":");
  return {
    name: host.name,
    color: host.color,
    connection: host.connection,
    address,
    port,
    projectDir: host.projectDir,
    installDir: host.installDir,
    manualIds: !!host.fileOwner,
    puid: host.fileOwner ? String(host.fileOwner.uid) : "",
    pgid: host.fileOwner ? String(host.fileOwner.gid) : "",
    detectSocket: true,
    socketGid: "",
    token,
    simulateFailure: false,
    reached: 0,
  };
}

/** Regenerated install files for an existing host with a fresh demo token. */
function InstallSheet({ host, onClose }: { host: Host | null; onClose: () => void }) {
  const t = useTranslations("hosts.install");
  const settings = useSettings();
  const cmd = useHostCommands();
  const [token, setToken] = useState<string | null>(null);
  const hostName = host?.name;
  useEffect(() => {
    if (!hostName) return;
    let live = true;
    void cmd.token(hostName).then((tk) => live && setToken(tk ?? null));
    return () => {
      live = false;
      setToken(null);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refresh only per host
  }, [hostName]);
  const general = settings.data?.general;
  return (
    <Sheet open={!!host} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full gap-0 p-0 sm:max-w-2xl">
        {host && (
          <>
            <SheetHeader className="border-b">
              <SheetTitle>{t("sheetTitle", { name: host.name })}</SheetTitle>
              <SheetDescription>{t("sheetDescription")}</SheetDescription>
            </SheetHeader>
            <ScrollArea className="min-h-0 flex-1">
              <div className="p-4">
                {token && general ? (
                  <InstallArtifacts plan={planFromDraft(draftOf(host, token), agentHubUrl(general), general.timeZone)} onRegenerateToken={() => void cmd.token(host.name).then((tk) => tk && setToken(tk))} />
                ) : (
                  <Skeleton className="h-80 rounded-xl" />
                )}
              </div>
            </ScrollArea>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

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
            <HostCard key={h.id} host={h} system={system.data?.find((c) => c.hostId === h.id)} {...count(h.id)} allIds={ids} onFiles={setFilesFor} />
          ))}
        </div>
      )}
      <InstallSheet host={filesFor} onClose={() => setFilesFor(null)} />
    </>
  );
}
