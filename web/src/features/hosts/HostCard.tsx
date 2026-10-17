import { Bot, Cable, ClipboardCheck, Eye, FileArchive, Filter, PlugZap, Settings2, Trash2, TriangleAlert, Unplug } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import { hostTone, isHostVisible } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { SYSTEM_LABEL, type SystemComponent } from "@/domain/system";
import { versionLabel } from "@/domain/updates";
import { BusyButton } from "@/shared/components/busy-button";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { cn } from "@/shared/lib/utils";
import { setPreference, useHostSelection } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { HostSettingsDialog } from "./HostSettingsDialog";
import { useHostAction, useHostCommands } from "./use-host-commands";

function Row({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 break-words", mono && "font-mono text-xs")}>{children}</dd>
    </>
  );
}

/** What the installer found locally on the host, as the agent reported it after connecting. */
function AgentReportSection({ host }: { host: Host }) {
  const t = useTranslations("hosts.card.report");
  const r = host.agentReport;
  return (
    <section aria-label={t("title")} className="rounded-xl border bg-card/40 p-3">
      <h3 className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
        <ClipboardCheck className="size-3.5" aria-hidden />
        {t("title")}
      </h3>
      {!r ? (
        <p className="text-xs text-muted-foreground">{t("pending")}</p>
      ) : (
        <>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <Row label={t("owner")} mono>
              {t("ownerValue", { name: r.projectDirOwner.name, uid: r.projectDirOwner.uid, gid: r.projectDirOwner.gid, writable: String(r.projectDirWritable) })}
            </Row>
            {r.fileOwner && r.fileOwnerSource ? <Row label={t("fileIds")} mono>
              {t("fileIdsValue", { uid: r.fileOwner.uid, gid: r.fileOwner.gid, source: r.fileOwnerSource })}
            </Row> : null}
            {r.socketGid !== undefined ? <Row label={t("socket")} mono>
              {t("socketValue", { gid: r.socketGid, source: r.socketGidSource ?? "detected" })}
            </Row> : null}
          </dl>
          {r.note !== "ok" && (
            <p className={cn("mt-1.5 flex items-start gap-1.5 text-xs", r.note === "root-owned" ? "text-warning" : "text-muted-foreground")}>
              {r.note === "root-owned" && <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />}
              {t(r.note === "root-owned" ? "root" : "unraid")}
            </p>
          )}
          <p className="mt-1.5 text-[11px] text-muted-foreground">{host.status !== "online" ? t("lastReported", { time: formatRelative(r.reportedAt) }) : t(r.simulated ? "reported" : "reportedLive", { time: formatRelative(r.reportedAt) })}</p>
        </>
      )}
    </section>
  );
}

/** Docklet's own agent on this host; shown here instead of in the stack and container lists. */
function SystemSection({ component }: { component: SystemComponent | undefined }) {
  const t = useTranslations("hosts.card.systemComponent");
  const ts = useTranslations("common.status");
  return (
    <section aria-label={t("title")} data-slot="system-component" className="flex items-start gap-2.5 rounded-xl border border-dashed bg-muted/15 p-3">
      <Bot className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
      {component ? (
        <div className="min-w-0 text-xs">
          <div className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
            {t("agent")}
            <span className="font-mono text-xs text-muted-foreground">{versionLabel(component.image)}</span>
            <ToneBadge tone={component.state === "running" ? "ok" : "orange"} className="h-5 px-1.5 text-[10.5px]">
              {ts(component.state === "running" ? "running" : "unknown")}
            </ToneBadge>
            <ToneBadge tone="gray" className="h-5 px-1.5 text-[10.5px]">
              {t("title")}
            </ToneBadge>
          </div>
          <p className="font-mono text-muted-foreground">{t("container", { name: component.containerName, project: component.composeProject })}</p>
          <p className="text-muted-foreground">{t(component.insideProjectDir ? "inside" : "outside", { label: `${SYSTEM_LABEL}=agent` })}</p>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">{t("none")}</p>
      )}
    </section>
  );
}

interface HostCardProps {
  host: Host;
  system: SystemComponent | undefined;
  stacks: number;
  containers: number;
  allIds: string[];
  onFiles?: (host: Host) => void;
}

/** One host with connection, directories, file IDs and actions; filter state stays visible. */
export function HostCard({ host, system, stacks, containers, allIds, onFiles }: HostCardProps) {
  const t = useTranslations("hosts");
  const tc = useTranslations("common.host");
  const dt = useAdapterText();
  const cmd = useHostCommands();
  const selection = useHostSelection();
  const [confirm, setConfirm] = useState(false);
  const [settings, setSettings] = useState(false);
  const visible = isHostVisible(selection, host.id);
  const removable = stacks === 0 && containers === 0;
  const pending = host.status === "pending";
  const connect = useHostAction(host.id, "onboard", () => cmd.connect(host.id));
  const check = useHostAction(host.id, "reconnect", () => cmd.reconnect(host.id));

  return (
    <Card
      data-host={host.id}
      style={{ "--host": host.color } as React.CSSProperties}
      className={cn("relative gap-4 overflow-hidden p-4", "before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-(--host)", !visible && "opacity-80")}
    >
      <header className="flex flex-wrap items-center gap-2">
        <span className="size-3 rounded-[4px] bg-(--host)" aria-hidden />
        <h2 className="text-lg font-semibold tracking-tight">{host.name}</h2>
        <ToneBadge tone={hostTone(host)}>
          <ToneDot tone={hostTone(host)} pulse={host.status !== "online"} className="size-1.5" />
          {tc(host.status)}
        </ToneBadge>
        <ToneBadge tone={host.connection === "edge" ? "info" : "accent"}>{t(`mode.${host.connection}`)}</ToneBadge>
        {host.demoAdded && <ToneBadge tone="violet">{t("card.demoAdded")}</ToneBadge>}
        <span className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button type="button" variant="ghost" size="icon" className="size-8 order-last" onClick={() => setSettings(true)} aria-label={t("settings.open", { name: host.name })} data-slot="host-settings-trigger">
              <Settings2 />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("settings.open", { name: host.name })}</TooltipContent>
        </Tooltip>
        {!visible && (
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <Filter className="size-3.5" aria-hidden />
            {t("filteredOut")}
            <Button variant="link" size="sm" className="h-auto px-1 text-xs" onClick={() => setPreference("hostSelection", selection.length + 1 >= allIds.length ? [] : [...selection, host.id])}>
              {t("showInFilter")}
            </Button>
          </span>
        )}
      </header>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
        <Row label={t("card.connection")}>{t(`mode.${host.connection}Long`)}</Row>
        <Row label={t("card.address")} mono>
          {dt(host.address)}
        </Row>
        <Row label={t("card.projectDir")} mono>
          {host.projectDir}
        </Row>
        <Row label={t("card.installDir")} mono>
          {host.installDir}
        </Row>
        {!host.agentReport && (
          <Row label={t("card.fileIds")} mono>
            {host.fileOwner ? `${host.fileOwner.uid}:${host.fileOwner.gid}` : t("card.fileIdsAuto")}
          </Row>
        )}
        <Row label={t("card.versions")} mono>
          {host.agentVersion} / {host.dockerVersion}
        </Row>
        <Row label={t("card.system")}>{host.os}</Row>
        <Row label={t("card.lastSeen")}>{host.status === "online" ? tc("online") : formatRelative(host.lastSeen)}</Row>
      </dl>
      <AgentReportSection host={host} />
      <SystemSection component={system} />
      <p className="text-xs text-muted-foreground">{t("card.usage", { stacks, containers })}</p>
      <footer className="flex flex-wrap gap-2 border-t pt-3">
        {pending ? (
          <BusyButton size="sm" busy={connect.busy} busyLabel={t("card.connecting")} icon={<PlugZap data-icon="inline-start" />} onClick={() => void connect.run()}>
            {t("card.connect")}
          </BusyButton>
        ) : (
          <BusyButton
            size="sm"
            variant="outline"
            className="group"
            busy={check.busy}
            busyLabel={t("card.checking")}
            icon={host.status === "offline" ? <Unplug data-icon="inline-start" /> : <Cable data-icon="inline-start" />}
            onClick={() => void check.run()}
          >
            {t("card.check")}
          </BusyButton>
        )}
        {onFiles && <Button size="sm" variant="outline" onClick={() => onFiles(host)}>
          <FileArchive data-icon="inline-start" />
          {t("card.files")}
        </Button>}
        <Button size="sm" variant="ghost" onClick={() => setPreference("hostSelection", allIds.length === 1 ? [] : [host.id])}>
          <Eye data-icon="inline-start" />
          {t("onlyThis")}
        </Button>
        <span className="flex-1" />
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button size="sm" variant="ghost" className="text-destructive hover:bg-destructive/10 hover:text-destructive focus-visible:ring-destructive/30" disabled={!removable} onClick={() => setConfirm(true)}>
                <Trash2 data-icon="inline-start" />
                {t("card.remove")}
              </Button>
            </span>
          </TooltipTrigger>
          {!removable && <TooltipContent>{t("card.removeBlocked")}</TooltipContent>}
        </Tooltip>
      </footer>
      <HostSettingsDialog host={host} component={system} open={settings} onOpenChange={setSettings} />
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("card.removeTitle", { name: host.name })}
        description={t("card.removeBody")}
        confirmLabel={t("card.removeConfirm")}
        destructive
        onConfirm={async () => (await cmd.remove(host.id)) ?? false}
      />
    </Card>
  );
}
