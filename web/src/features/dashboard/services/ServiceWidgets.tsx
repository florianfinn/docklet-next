import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, ExternalLink, MonitorPlay, Plug, Power, TriangleAlert, Users, X } from "lucide-react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { useCommandError } from "@/data/use-command-error";
import { useServiceWidget } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import type { MediaRequest, ServiceKind, WidgetResult, WidgetSummary } from "@/domain/service-widgets";
import { QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";

import { SERVICE_COLOR, widgetErrorKey } from "./widget-meta";

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

function Notice({ icon, title, body, action }: { icon: React.ReactNode; title: string; body: string; action?: React.ReactNode }) {
  return (
    <div className="grid h-full place-items-center text-center" data-widget-notice>
      <div className="flex max-w-xs flex-col items-center gap-2">
        {icon}
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{body}</p>
        {action}
      </div>
    </div>
  );
}

function PlexBody({ data }: { data: NonNullable<WidgetResult["plex"]> }) {
  const t = useTranslations("dashboard.services.plex");
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <MonitorPlay className="size-3.5" aria-hidden />
        {t("summary", { count: data.streams, transcodes: data.transcodes })}
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto" aria-label={t("streams")}>
        {data.sessions.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t("none")}</li>}
        {data.sessions.map((s, i) => (
          <li key={`${s.user}:${s.title}:${i}`} className="flex items-center gap-2.5 rounded-xl border bg-muted/15 p-2.5">
            <Avatar name={s.user} />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5 text-sm">
                <span className="truncate font-medium">{s.title}</span>
                <span className="shrink-0 text-[11px] text-muted-foreground">· {s.user}</span>
              </div>
              <div className="truncate text-[11px] text-muted-foreground">{[s.subtitle, s.device].filter(Boolean).join(" · ")}</div>
              <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={s.progress} aria-valuemin={0} aria-valuemax={100} aria-label={t("progress", { title: s.title })}>
                <span className="block h-full rounded-full transition-[width] duration-(--dur-slow)" style={{ width: `${s.progress}%`, background: SERVICE_COLOR.plex }} />
              </div>
            </div>
            <div className="flex shrink-0 flex-col items-end gap-1">
              {s.quality && <span className="rounded border px-1 font-mono text-[10.5px] text-muted-foreground">{s.quality}</span>}
              <ToneBadge tone={s.transcode ? "warn" : "ok"} className="h-5 px-1.5 text-[10.5px]">
                {t(s.transcode ? "transcode" : "direct")}
              </ToneBadge>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

function MinecraftBody({ data }: { data: NonNullable<WidgetResult["minecraft"]> }) {
  const t = useTranslations("dashboard.services.minecraft");
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-end justify-between gap-3 rounded-xl border bg-muted/15 p-3">
        <div>
          <div className="text-2xl font-semibold tabular-nums" style={{ color: SERVICE_COLOR.minecraft }}>
            {data.online}
            <span className="text-sm font-normal text-muted-foreground">/{data.max}</span>
          </div>
          <div className="text-xs text-muted-foreground">{t("online")}</div>
        </div>
        <div className="min-w-0 text-right text-[11px] text-muted-foreground">
          {data.motd && <div className="truncate font-medium text-foreground">{data.motd}</div>}
          {data.version && <div>{t("version", { version: data.version })}</div>}
        </div>
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-auto" aria-label={t("players")}>
        {data.players.map((name) => (
          <li key={name} className="flex items-center gap-2.5 rounded-lg px-1.5 py-1 hover:bg-muted/30">
            <Avatar name={name} square />
            <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{name}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Request ages count from the hub fetch time, so a render never moves them. */
function OverseerrBody({ data, canDecide, now }: { data: NonNullable<WidgetResult["overseerr"]>; canDecide: boolean; now: number }) {
  const t = useTranslations("dashboard.services.overseerr");
  const adapter = useAdapter();
  const client = useQueryClient();
  const commandError = useCommandError();
  const decide = useMutation({
    mutationFn: ({ request, decision }: { request: MediaRequest; decision: "approve" | "decline" }) => adapter.decideMediaRequest(request.id, decision),
    onSuccess: (_, { request, decision }) => toast.success(t(decision === "approve" ? "approvedToast" : "declinedToast", { title: request.title })),
    onError: (error) => toast.error(commandError(error)),
    onSettled: () => client.invalidateQueries({ queryKey: queryKeys.serviceWidget("overseerr") }),
  });
  return (
    <div className="flex h-full flex-col gap-2.5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <ToneBadge tone={data.pending ? "warn" : "ok"} className="h-5 px-1.5 text-[10.5px]">
          {t("pending", { count: data.pending })}
        </ToneBadge>
        {t("other", { approved: data.approved, available: data.available })}
      </div>
      <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto" aria-label={t("requests")}>
        {data.requests.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t("none")}</li>}
        {data.requests.map((r) => (
          <li key={r.id} className="flex items-center gap-2.5 rounded-xl border bg-muted/15 p-2.5">
            <Avatar name={r.requestedBy || r.title} />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{r.title}</div>
              <div className="truncate text-[11px] text-muted-foreground">{t("meta", { kind: r.mediaType, user: r.requestedBy, hours: Math.max(0, Math.floor((now - Date.parse(r.createdAt)) / 3_600_000)) })}</div>
            </div>
            {canDecide && (
              <>
                <Button size="icon" variant="ghost" className="size-8 text-success hover:text-success" disabled={decide.isPending} onClick={() => decide.mutate({ request: r, decision: "approve" })} aria-label={t("approve", { title: r.title })}>
                  <Check />
                </Button>
                <Button size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" disabled={decide.isPending} onClick={() => decide.mutate({ request: r, decision: "decline" })} aria-label={t("decline", { title: r.title })}>
                  <X />
                </Button>
              </>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function Links({ summary, name }: { summary: WidgetSummary; name: string }) {
  const t = useTranslations("dashboard.services");
  if (!summary.openUrl && !summary.stackId) return null;
  return (
    <div className="flex flex-wrap gap-1">
      {summary.openUrl && (
        <Button asChild size="sm" variant="ghost" className="text-xs">
          <a href={summary.openUrl} target="_blank" rel="noopener noreferrer">
            <ExternalLink data-icon="inline-start" />
            {t("open", { name })}
          </a>
        </Button>
      )}
      {summary.stackId && (
        <Button asChild size="sm" variant="ghost" className="text-xs">
          <Link to={`/stacks/${encodeURIComponent(summary.stackId)}`}>
            <Users data-icon="inline-start" />
            {t("openStack")}
          </Link>
        </Button>
      )}
    </div>
  );
}

/**
 * Body of a service widget from the hub: not set up and turned off (administrators only, with a
 * link to the settings), loading, failure, stale data with its age, and data.
 */
export function ServiceWidgetBody({ kind, summary, admin }: { kind: ServiceKind; summary: WidgetSummary; admin: boolean }) {
  const t = useTranslations("dashboard.services");
  const active = summary.configured && summary.enabled;
  const query = useServiceWidget(kind, active);
  const setUp = admin ? (
    <Button asChild size="sm" variant="outline">
      <Link to="/settings/serviceWidgets">{t("setUp")}</Link>
    </Button>
  ) : undefined;
  if (!summary.configured) return <Notice icon={<Plug className="size-6 text-muted-foreground" aria-hidden />} title={t("notConfigured")} body={t("notConfiguredBody")} action={setUp} />;
  if (!summary.enabled) return <Notice icon={<Power className="size-6 text-muted-foreground" aria-hidden />} title={t("disabled")} body={t("disabledBody")} action={setUp} />;
  if (query.isPending) return <Rows />;
  if (query.isError) return <QueryError error={query.error} compact onRetry={() => void query.refetch()} />;
  const result = query.data;
  const hasData = !!(result.plex || result.overseerr || result.minecraft);
  if (!hasData) return <Notice icon={<TriangleAlert className="size-6 text-destructive" aria-hidden />} title={t("failed")} body={result.error ? t(widgetErrorKey(result.error)) : t("errors.bad-response")} action={result.error?.code === "widget.invalid-config" || result.error?.code === "widget.credential-unavailable" ? setUp : undefined} />;
  const name = t(`${kind}.name`);
  return (
    <div className="flex h-full flex-col gap-2" data-widget-state={result.stale ? "stale" : "data"}>
      {result.stale && (
        <p className="flex items-center gap-1.5 text-[11px] text-warning" role="status">
          <TriangleAlert className="size-3.5" aria-hidden />
          {t("stale", { minutes: Math.max(1, Math.round(result.ageSeconds / 60)) })}
          {result.error && <span className="text-muted-foreground">· {t(widgetErrorKey(result.error))}</span>}
        </p>
      )}
      <div className="min-h-0 flex-1">
        {result.plex && <PlexBody data={result.plex} />}
        {result.minecraft && <MinecraftBody data={result.minecraft} />}
        {result.overseerr && <OverseerrBody data={result.overseerr} canDecide={admin} now={result.fetchedAt ? Date.parse(result.fetchedAt) : query.dataUpdatedAt} />}
      </div>
      <Links summary={summary} name={name} />
    </div>
  );
}
