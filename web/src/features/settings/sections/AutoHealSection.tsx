// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original per-host Auto Heal overview with read-only and unavailable states.

import { HeartPulse, Pencil } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { useTranslations } from "use-intl";

import { notificationKeys, useAutoHeal } from "@/data/notification-queries";
import { useCapability } from "@/data/use-capability";
import { settingsErrorMessage } from "@/data/settings-error";
import type { AutoHealSettings } from "@/domain/auto-heal";
import type { Host } from "@/domain/model";
import { QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";

import { AutoHealDialog } from "../auto-heal/AutoHealDialog";

interface DialogState {
  generation: number;
  host: Host;
  settings: AutoHealSettings;
}

export function AutoHealSection() {
  const t = useTranslations("settings.autoHeal");
  const ts = useTranslations("settings");
  const hosts = useHostScope();
  const client = useQueryClient();
  const generations = useRef(0);
  const [dialog, setDialog] = useState<DialogState>();
  const close = (generation: number) => setDialog((current) => (current?.generation === generation ? undefined : current));
  return (
    <section className="flex flex-col gap-4 rounded-xl border bg-card p-6" aria-labelledby="autoheal-title">
      <div className="min-w-0">
        <h2 id="autoheal-title" className="flex items-center gap-2 text-lg font-semibold">
          <HeartPulse className="size-4 text-destructive" aria-hidden />
          {ts("sections.autoHeal")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("scopeHint")}</p>
      </div>
      {hosts.isError ? (
        <QueryError error={hosts.error} onRetry={() => void hosts.refetch()} />
      ) : hosts.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">{t("loading")}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {hosts.all.map((host) => (
            <HostRow key={host.id} host={host} edit={(settings) => setDialog({ generation: ++generations.current, host, settings })} />
          ))}
        </ul>
      )}
      {dialog && (
        <AutoHealDialog
          key={dialog.generation}
          host={dialog.host}
          settings={dialog.settings}
          onClose={() => close(dialog.generation)}
          onSaved={() => {
            void client.invalidateQueries({ queryKey: notificationKeys.autoHeal(dialog.host.id) });
            close(dialog.generation);
          }}
        />
      )}
    </section>
  );
}

function Unavailable({ host, reason }: { host: Host; reason: string }) {
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-xl border border-dashed p-3">
      <span className="min-w-0 flex-1 font-medium">{host.name}</span>
      <span className="text-xs text-muted-foreground">{reason}</span>
    </li>
  );
}

function HostRow({ host, edit }: { host: Host; edit: (settings: AutoHealSettings) => void }) {
  const t = useTranslations("settings.autoHeal");
  const all = useTranslations();
  const online = host.status === "online";
  const capability = useCapability("getAutoHealSettings", { kind: "host", id: host.id });
  const query = useAutoHeal(host.id, online && capability.available);
  if (!online) return <Unavailable host={host} reason={t(`hostStatus.${host.status}`)} />;
  if (!capability.available) return <Unavailable host={host} reason={capability.pending ? t("loading") : capability.reason!} />;
  if (query.isError) return <Unavailable host={host} reason={settingsErrorMessage(query.error, all)} />;
  const s = query.data;
  const readOnly = s?.managedByEnvironment ? t("managedByEnvironment") : s?.configurationLocked ? t("configurationLocked") : undefined;
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{host.name}</span>
          {s && <ToneBadge tone={s.enabled ? "ok" : "gray"} className="h-5 px-1.5 text-[10.5px]">{s.enabled ? t("on") : t("off")}</ToneBadge>}
        </div>
        {!s ? (
          <p role="status" className="text-xs text-muted-foreground">{t("loading")}</p>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t("summary", { count: s.maxRestarts, minutes: s.windowMinutes, stacks: s.excludedStacks.length, containers: s.excludedContainers.length })}
            {readOnly && ` · ${readOnly}`}
          </p>
        )}
      </div>
      {s && !readOnly && (
        <Button type="button" size="sm" variant="outline" aria-label={`${t("edit")}: ${host.name}`} onClick={() => edit(s)}>
          <Pencil data-icon="inline-start" />
          {t("edit")}
        </Button>
      )}
    </li>
  );
}
