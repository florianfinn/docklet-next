import { TailnetBadge, TailnetHostCause, TailnetStatusTooltip } from "@/features/connections/TailnetHosts";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { Bot, Cable, Eye, FileArchive, Filter, PlugZap, Settings2, Trash2, Unplug } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { useCapability } from "@/data/use-capability";
import { RotateTokenButton } from "./RotateTokenButton";
import { formatRelative } from "@/domain/format";
import { hostTone, isHostVisible, isHostAvailable } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { SYSTEM_LABEL, type SystemComponent } from "@/domain/system";
import { versionLabel } from "@/domain/updates";
import { BusyButton } from "@/shared/components/busy-button";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { cn } from "@/shared/lib/utils";
import { setPreference, useHostSelection, usePreference } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Switch } from "@/shared/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { SystemMetadata } from "./SystemMetadata";
import { AgentReportSection } from "./AgentReportSection";
import { HostSettingsDialog } from "./HostSettingsDialog";
import { useHostAction, useHostCommands } from "./use-host-commands";
import { useHostDetails } from "./use-host-details";
import { unraidDetected } from "./unraid-detection";
import { identityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { unreadableRows } from "@/domain/unreadable";

function Row({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 break-words", mono && "font-mono text-xs")}>{children}</dd>
    </>
  );
}

/** Docklet's own agent on this host; shown here instead of in the stack and container lists. */
function SystemSection({ component }: { component: SystemComponent | undefined }) {
  const t = useTranslations("hosts.card.systemComponent");
  const ts = useTranslations("common.status");
  if (component?.metadataOnly) return <SystemMetadata component={component} />;
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

/** Per-user choice whether this host's Unraid-managed containers count; shown only where Unraid was detected or a choice is remembered. */
function UnraidSwitch({ hostId, count }: { hostId: string; count: number | undefined }) {
  const t = useTranslations("hosts.card");
  const hidden = usePreference("unraidHidden");
  const excluded = hidden.includes(hostId);
  const identity = identityLifetime(useAdapter()).version();
  const id = `unraid-${hostId}`;
  if (!unraidDetected(identity, hostId, count) && !excluded) return null;
  return (
    <div data-slot="unraid-switch" className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      <Switch id={id} checked={!excluded} onCheckedChange={(on) => setPreference("unraidHidden", on ? hidden.filter((h) => h !== hostId) : [...hidden, hostId])} />
      <label htmlFor={id}>{t("unraidInclude")}</label>
      {!!count && <span className="text-xs text-muted-foreground">{t("unraidCount", { count })}</span>}
    </div>
  );
}

interface HostCardProps {
  host: Host;
  system: SystemComponent | undefined;
  stacks?: number;
  containers?: number;
  /** Unraid-managed containers from a fresh, complete inventory of this host; undefined while unknown. */
  unraid?: number;
  allIds: string[];
  onFiles?: (host: Host) => void;
}

/** One host with connection, directories, file IDs and actions; filter state stays visible. */
export function HostCard({ host, system, stacks, containers, unraid, allIds, onFiles }: HostCardProps) {
  const unreadable = unreadableRows(host.inventory, ["stacks", "containers"]).count;
  const t = useTranslations("hosts");
  const tn = useTranslations("connections");
  const tc = useTranslations("common.host");
  const dt = useAdapterText();
  // The hub's own Docker (environment "0") has no agent to report or connect.
  const local = host.id === "0";
  const cmd = useHostCommands();
  const reconnect = useCapability("reconnectHost", { kind: "host", id: host.id });
  const connection = useCapability("connectHost", { kind: "host", id: host.id });
  const selection = useHostSelection();
  const [confirm, setConfirm] = useState(false);
  const [settings, setSettings] = useState(false);
  const visible = isHostVisible(selection, host.id);
  const remove = useCapability("removeHost");
  const removable = remove.available && host.id !== "0";
  const pending = host.status === "pending";
  const connect = useHostAction(host.id, "onboard", () => cmd.connect(host.id));
  const check = useHostAction(host.id, "reconnect", () => cmd.reconnect(host.id));
  const details = useHostDetails(host);
  const os = details?.os || host.os, dockerVersion = details?.dockerVersion || host.dockerVersion;
  // The hub knows its own project directory from its settings; agents report theirs.
  const projectDir = local ? details?.projectDir : host.projectDir;

  return (
    <Card
      data-host={host.id}
      style={{ "--host": host.color } as React.CSSProperties}
      className={cn("relative gap-4 overflow-hidden p-4", "before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-(--host)", !visible && "opacity-80")}
    >
      <header className="flex flex-wrap items-center gap-2">
        <span className="size-3 rounded-[4px] bg-(--host)" aria-hidden />
        <h2 className="text-lg font-semibold tracking-tight">{host.name}</h2>
        <TailnetStatusTooltip host={host}><ToneBadge tone={hostTone(host)}>
          <ToneDot tone={hostTone(host)} pulse={host.status !== "online"} className="size-1.5" />
          {tc(host.status)}
        </ToneBadge></TailnetStatusTooltip>
        <ToneBadge tone={host.connection === "edge" ? "info" : "accent"}>{t(`mode.${host.connection}`)}</ToneBadge>
        {host.networkRoute === "tailnet" && <TailnetBadge />}
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
        <Row label={t("card.connection")}>{host.networkRoute === "tailnet" ? tn("tailnet") : t(local ? "mode.localLong" : `mode.${host.connection}Long`)}</Row>
        <Row label={t(local ? "card.hubAddress" : "card.address")} mono>
          {host.networkRoute === "tailnet" ? <><span>{[host.tailnet?.dnsName, host.tailnet?.addresses?.find((address) => !address.includes(":")), tn("name")].filter(Boolean).join(" · ")}</span><p className="mt-1 font-sans text-xs text-muted-foreground">{tn("routeHint")}</p></> : dt(host.address) || t(host.connection === "edge" ? "card.addressNotRequired" : "card.addressNotConfigured")}
        </Row>
        <Row label={t("card.projectDir")} mono>
          {projectDir || t("card.unavailable")}
        </Row>
        {!local && <Row label={t("card.installDir")} mono>
          {host.installDir || t("card.unavailable")}
        </Row>}
        {!local && !host.agentReport && (
          <Row label={t("card.fileIds")} mono>
            {host.fileOwner ? `${host.fileOwner.uid}:${host.fileOwner.gid}` : t("card.fileIdsAuto")}
          </Row>
        )}
        {local ? <Row label={t("card.docker")} mono>{dockerVersion || t("card.unavailable")}</Row> : <Row label={t(dockerVersion ? "card.versions" : "card.agentVersion")} mono>
          {[host.agentVersion === "dev" ? t("card.development") : host.agentVersion, dockerVersion].filter(Boolean).join(" / ") || t("card.unavailable")}
        </Row>}
        <Row label={t("card.system")}>{os || t("card.unavailable")}</Row>
        <Row label={t("card.lastSeen")}>{host.status === "online" ? tc("online") : host.lastSeen ? formatRelative(host.lastSeen) : t("card.neverContacted")}{host.tailnet && ["direct", "relay", "idle"].includes(host.tailnet.path) && <> · {tn(`path.${host.tailnet.path as "direct" | "relay" | "idle"}`)}</>}</Row>
      </dl>
      <TailnetHostCause host={host} />
      {host.status === "token-rejected" && <p role="status" className="text-sm text-destructive">{t("tokenRejected")}</p>}
      {!local && <AgentReportSection host={host} />}
      {(system || !host.agentVersion && !host.agentReport) && <CapabilityGate operation="listSystemComponents" resourceRef={{ kind: "host", id: host.id }}><SystemSection component={system} /></CapabilityGate>}
      <p className="text-xs text-muted-foreground">{stacks === undefined || containers === undefined ? t("card.usageUnknown") : t("card.usage", { stacks, containers })}{unreadable > 0 && <span className="text-warning"> · {t("card.unreadable", { count: unreadable })}</span>}</p>
      <UnraidSwitch hostId={host.id} count={unraid} />
      <footer className="flex flex-wrap gap-2 border-t pt-3">
        {pending ? (
          <BusyButton size="sm" disabled={!connection.available} title={connection.reason} busy={connect.busy} busyLabel={t("card.connecting")} icon={<PlugZap data-icon="inline-start" />} onClick={() => void connect.run()}>
            {t("card.connect")}
          </BusyButton>
        ) : (
          <BusyButton
            size="sm"
            variant="outline"
            className="group"
            disabled={!reconnect.available} title={reconnect.reason} busy={check.busy}
            busyLabel={t("card.checking")}
            icon={host.status === "offline" ? <Unplug data-icon="inline-start" /> : <Cable data-icon="inline-start" />}
            onClick={() => void check.run()}
          >
            {t("card.check")}
          </BusyButton>
        )}
        <RotateTokenButton host={host} />
        {onFiles && host.networkRoute !== "tailnet" && host.id !== "0" && <Button size="sm" variant="outline" onClick={() => onFiles(host)}>
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
        description={`${t("card.removeCounts", { stacks: isHostAvailable(host) && !host.inventory?.stale ? String(stacks ?? t("card.unknown")) : t("card.unknown"), containers: isHostAvailable(host) && !host.inventory?.stale ? String(containers ?? t("card.unknown")) : t("card.unknown") })} ${t("card.removeBody")}`}
        confirmLabel={t("card.removeConfirm")}
        destructive
        onConfirm={async () => (await cmd.remove(host.id)) ?? false}
      />
    </Card>
  );
}
