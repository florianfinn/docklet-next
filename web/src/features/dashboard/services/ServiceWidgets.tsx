import { Check, MonitorPlay, Plug, Settings2, TriangleAlert, Users, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { formatNumber } from "@/domain/locale";
import { ToneBadge } from "@/shared/components/tone";
import { useDemoText } from "@/shared/hooks/use-demo-text";
import { useStore } from "@/shared/lib/store";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";

import type { ServiceConfig, ServiceKind } from "../dashboard-layout";
import { connectingStore, MINECRAFT, OVERSEERR_REQUESTS, PLEX_STREAMS, SERVICE_META, streamProgress, type MediaRequest } from "./service-data";


function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

/** Deterministic tint per name for the small avatars. */
function hue(name: string): number {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) % 360;
  return h;
}

function Avatar({ name, square }: { name: string; square?: boolean }) {
  return (
    <span className={square ? "grid size-7 shrink-0 place-items-center rounded-[5px] text-[11px] font-bold text-white" : "grid size-7 shrink-0 place-items-center rounded-full text-[11px] font-semibold text-white"} style={{ background: `oklch(0.55 0.13 ${hue(name)})` }} aria-hidden>
      {name.slice(0, 1).toUpperCase()}
    </span>
  );
}

function Rows() {
  return (
    <div className="flex h-full flex-col gap-2" aria-busy="true">
      <Skeleton className="h-9 rounded-lg" />
      {Array.from({ length: 4 }, (_, i) => (
        <Skeleton key={i} className="h-12 rounded-lg" />
      ))}
    </div>
  );
}

function PlexBody() {
  const t = useTranslations("dashboard.services.plex");
  const dt = useDemoText();
  const now = useNow(5000);
  const transcodes = PLEX_STREAMS.filter((s) => s.mode === "transcode").length;
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <MonitorPlay className="size-3.5" aria-hidden />
        {t("summary", { count: PLEX_STREAMS.length, transcodes })}
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto" aria-label={t("streams")}>
        {PLEX_STREAMS.map((s) => {
          const p = streamProgress(s, now);
          return (
            <li key={s.id} className="flex items-center gap-2.5 rounded-xl border bg-muted/15 p-2.5">
              <Avatar name={s.user} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5 text-sm">
                  <span className="truncate font-medium">{s.title}</span>
                  <span className="shrink-0 text-[11px] text-muted-foreground">· {s.user}</span>
                </div>
                <div className="truncate text-[11px] text-muted-foreground">
                  {dt(s.subtitle)} · {dt(s.device)}
                </div>
                <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={p} aria-valuemin={0} aria-valuemax={100} aria-label={t("progress", { title: s.title })}>
                  <span className="block h-full rounded-full transition-[width] duration-(--dur-slow)" style={{ width: `${p}%`, background: SERVICE_META.plex.color }} />
                </div>
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1">
                <span className="rounded border px-1 font-mono text-[10.5px] text-muted-foreground">{s.quality}</span>
                <ToneBadge tone={s.mode === "direct" ? "ok" : "warn"} className="h-5 px-1.5 text-[10.5px]">
                  {t(s.mode)}
                </ToneBadge>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function MinecraftBody() {
  const t = useTranslations("dashboard.services.minecraft");
  const m = MINECRAFT;
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-end justify-between gap-3 rounded-xl border bg-muted/15 p-3">
        <div>
          <div className="text-2xl font-semibold tabular-nums" style={{ color: SERVICE_META.minecraft.color }}>
            {m.players.length}
            <span className="text-sm font-normal text-muted-foreground">/{m.max}</span>
          </div>
          <div className="text-xs text-muted-foreground">{t("online")}</div>
        </div>
        <div className="text-right text-[11px] text-muted-foreground">
          <div className="font-medium text-foreground">{m.motd}</div>
          <div>
            {m.version} · {t("tps", { tps: formatNumber(m.tps) })}
          </div>
        </div>
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-auto" aria-label={t("players")}>
        {m.players.map((p) => (
          <li key={p.name} className="flex items-center gap-2.5 rounded-lg px-1.5 py-1 hover:bg-muted/30">
            <Avatar name={p.name} square />
            <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{p.name}</span>
            <span className="text-[11px] text-muted-foreground tabular-nums">{t("session", { minutes: p.minutes })}</span>
          </li>
        ))}
      </ul>
      {SERVICE_META.minecraft.stackId && (
        <Button asChild size="sm" variant="ghost" className="self-start text-xs">
          <Link to={`/stacks/${SERVICE_META.minecraft.stackId}`}>
            <Users data-icon="inline-start" />
            {t("stack")}
          </Link>
        </Button>
      )}
    </div>
  );
}

function OverseerrBody() {
  const t = useTranslations("dashboard.services.overseerr");
  const [requests, setRequests] = useState<MediaRequest[]>(OVERSEERR_REQUESTS);
  const pending = requests.filter((r) => r.status === "pending");
  const decide = (r: MediaRequest, status: "approved" | "declined") => {
    setRequests((list) => list.map((x) => (x.id === r.id ? { ...x, status } : x)));
    toast.success(t(status === "approved" ? "approvedToast" : "declinedToast", { title: r.title }));
  };
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <ToneBadge tone={pending.length ? "warn" : "ok"} className="h-5 px-1.5 text-[10.5px]">
          {t("pending", { count: pending.length })}
        </ToneBadge>
        {t("other", { approved: requests.filter((r) => r.status === "approved").length, available: requests.filter((r) => r.status === "available").length })}
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto" aria-label={t("requests")}>
        {pending.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t("none")}</li>}
        {pending.map((r) => (
          <li key={r.id} className="flex items-center gap-2.5 rounded-xl border bg-muted/15 p-2.5">
            <Avatar name={r.user} />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{r.title}</div>
              <div className="truncate text-[11px] text-muted-foreground">{t("meta", { kind: r.kind, user: r.user, hours: r.ageHours })}</div>
            </div>
            <Button size="icon" variant="ghost" className="size-8 text-success hover:text-success" onClick={() => decide(r, "approved")} aria-label={t("approve", { title: r.title })}>
              <Check />
            </Button>
            <Button size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" onClick={() => decide(r, "declined")} aria-label={t("decline", { title: r.title })}>
              <X />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Body of a service widget by connection state; content is synthetic. */
export function ServiceWidgetBody({ kind, config, onConfigure }: { kind: ServiceKind; config: ServiceConfig; onConfigure: () => void }) {
  const t = useTranslations("dashboard.services");
  const connecting = useStore(connectingStore, (s) => s.includes(kind));
  if (connecting) return <Rows />;
  if (config.status === "unconfigured")
    return (
      <div className="grid h-full place-items-center text-center">
        <div className="flex max-w-xs flex-col items-center gap-2">
          <Plug className="size-6 text-muted-foreground" aria-hidden />
          <p className="text-sm font-medium">{t("missing")}</p>
          <p className="text-xs text-muted-foreground">{t("missingBody")}</p>
          <Button size="sm" onClick={onConfigure}>
            <Settings2 data-icon="inline-start" />
            {t("configure")}
          </Button>
        </div>
      </div>
    );
  if (config.status === "error")
    return (
      <div className="grid h-full place-items-center text-center">
        <div className="flex max-w-xs flex-col items-center gap-2">
          <TriangleAlert className="size-6 text-destructive" aria-hidden />
          <p className="text-sm font-medium">{t("error")}</p>
          <p className="text-xs text-muted-foreground">{t("errorBody", { address: config.address })}</p>
          <Button size="sm" variant="outline" onClick={onConfigure}>
            <Settings2 data-icon="inline-start" />
            {t("configure")}
          </Button>
        </div>
      </div>
    );
  if (kind === "plex") return <PlexBody />;
  if (kind === "minecraft") return <MinecraftBody />;
  return <OverseerrBody />;
}
