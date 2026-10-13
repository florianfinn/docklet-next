import { useTranslations } from "use-intl";

import { hostTone } from "@/domain/hosts";
import type { Host } from "@/domain/model";
import { ToneDot } from "@/shared/components/tone";

/**
 * Host a network dialog acts on: identity square, name and status in the
 * host's own colour, with a one-line scope hint. The colour only identifies;
 * every check still compares host ids (checkConnect, validateNetworkDraft).
 */
export function HostScope({ host, hint }: { host: Host | undefined; hint: string }) {
  const t = useTranslations("networks.scope");
  const tc = useTranslations("common.host");
  if (!host) return null;
  return (
    <div style={{ "--host": host.color } as React.CSSProperties} className="flex flex-wrap items-center gap-x-2.5 gap-y-1 border-l-2 border-(--host)/70 pl-2.5" data-slot="host-scope" data-host={host.id}>
      <span className="inline-flex h-6 items-center gap-1.5 rounded-full border border-(--host)/40 bg-(--host)/10 px-2.5 text-xs font-medium text-foreground">
        <span className="size-2.5 rounded-[3px] bg-(--host)" aria-hidden />
        <span className="sr-only">{t("label")}: </span>
        {host.name}
        {host.status !== "online" && <ToneDot tone={hostTone(host)} label={tc(host.status)} className="size-1.5" />}
      </span>
      <span className="text-xs text-muted-foreground">{hint}</span>
    </div>
  );
}
