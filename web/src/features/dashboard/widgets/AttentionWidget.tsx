import { useCapability } from "@/data/use-capability";
import { Boxes, HardDrive, History, PartyPopper, ShieldAlert, Siren, Sparkles, Unplug } from "lucide-react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { formatRelative } from "@/domain/format";
import type { Host } from "@/domain/model";
import { HostChip } from "@/shared/components/host-chip";
import { IconTile } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { ScrollArea } from "@/shared/ui/scroll-area";

import { AttentionServiceRestart } from "./AttentionServiceRestart";
import type { AttentionItem, AttentionKind } from "../attention";
import { useAttentionText } from "../attention-text";

const ICON: Record<AttentionKind, ReactNode> = {
  unhealthy: <Siren />,
  offline: <Unplug />,
  failed: <History />,
  critical: <ShieldAlert />,
  major: <Sparkles />,
  disk: <HardDrive />,
  unknownOrigin: <Boxes />,
};

/** Ranked triage queue; each row offers the one action that resolves it. */
export function AttentionWidget({ items, hosts }: { items: AttentionItem[]; hosts: Map<string, Host> }) {
  const t = useTranslations("dashboard.attention");
  const title = useAttentionText();
  const navigate = useNavigate();

  const sub = (item: AttentionItem) => {
    const v = item.values;
    switch (item.kind) {
      case "unhealthy":
        return t("unhealthySub", { stack: String(v.stack), restarts: Number(v.restarts) });
      case "offline":
        return t("offlineSub", { time: formatRelative(String(v.time)), containers: Number(v.containers ?? 0) });
      case "failed":
        return t("failedSub");
      case "critical":
        return t("criticalSub", { count: Number(v.count) });
      case "major":
        return t("majorSub", { current: String(v.current), latest: String(v.latest) });
      case "disk":
        return t("diskSub", { percent: Number(v.percent), days: Number(v.days) });
      default:
        return t("unknownOriginSub");
    }
  };

  const actions = (item: AttentionItem) => {
    switch (item.kind) {
      case "unhealthy":
        return (
          <>
            <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={() => navigate(`/stacks/${item.stackId}?tab=logs&service=${item.service}`)}>
              {t("actions.logs")}
            </Button>
            <AttentionServiceRestart item={item} />
          </>
        );
      case "offline":
        return (
          <HostCheckButton hostId={item.hostId} label={t("actions.check")} />
        );
      case "failed":
        return (
          <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => navigate(`/stacks/${item.stackId}?tab=deployments`)}>
            {t("actions.restore")}
          </Button>
        );
      case "critical":
        return (
          <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => navigate("/security?tab=findings")}>
            {t("actions.security")}
          </Button>
        );
      case "major":
        return (
          <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => navigate(`/stacks/${item.stackId}`)}>
            {t("actions.open")}
          </Button>
        );
      case "disk":
        return null;
      default:
        return (
          <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={() => navigate(`/containers?c=${item.containerId}`)}>
            {t("actions.assign")}
          </Button>
        );
    }
  };

  if (!items.length) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-sm text-muted-foreground">
        <IconTile tone="ok" size="lg">
          <PartyPopper />
        </IconTile>
        {t("empty")}
      </div>
    );
  }
  return (
    <ScrollArea className="h-full">
      <ol className="flex flex-col gap-2 pr-2">
        {items.map((item) => (
          <li key={item.id} data-tone={item.tone} className="group relative flex items-start gap-3 overflow-hidden rounded-xl border border-(--tone)/25 bg-(--tone)/6 p-2.5">
            <span aria-hidden className="absolute inset-y-0 left-0 w-0.5 bg-(--tone)" />
            <IconTile tone={item.tone} size="sm" className="tilt mt-0.5">
              {ICON[item.kind]}
            </IconTile>
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{title(item)}</div>
              <div className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                <span className="truncate">{sub(item)}</span>
                <HostChip host={hosts.get(item.hostId ?? "")} />
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1 self-center">{actions(item)}</div>
          </li>
        ))}
      </ol>
    </ScrollArea>
  );
}

function HostCheckButton({ hostId, label }: { hostId?: string; label: string }) {
  const adapter = useAdapter();
  const capability = useCapability("reconnectHost", { kind: "host", id: hostId ?? "" });
  return <Button size="sm" variant="outline" className="h-7 px-2 text-xs" disabled={!hostId || !capability.available} title={capability.reason} onClick={() => hostId && void adapter.reconnectHost(hostId)}>{label}</Button>;
}
