import { useCapability } from "@/data/use-capability";
import { ArrowRight, Box, KeyRound, RefreshCw } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { formatDate, formatDateTime, formatRelative } from "@/domain/format";
import type { ImageVersion, Service, UpdateInfo } from "@/domain/model";
import { hasUpdate, summarizeUpdates, updateVisual } from "@/domain/updates";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";

import { LuminousIcon, ToneBadge } from "../tone";
import { checkTimes, type UpdateTimes } from "./update-times";
import { latestOf, UPDATE_ICON, UPDATE_TONE, useUpdateCopy } from "./update-visuals";

export interface UpdateActions {
  onUpdate?: () => void;
  updateOperation?: "installStackUpdates" | "installContainerUpdate";
  onRecheck?: () => void;
  updateLabel?: string;
  /** Explains why updating is not possible here (e.g. Unraid-managed). */
  blockedReason?: string;
  busy?: boolean;
  extra?: ReactNode;
}

function Header({ tone, icon, title, desc, chip }: { tone: string; icon: ReactNode; title: string; desc: string; chip?: string }) {
  return (
    <div data-tone={tone} className="tone-surface flex items-start gap-3 p-4">
      <LuminousIcon tone={tone as never}>{icon}</LuminousIcon>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold text-(--tone)">{title}</div>
        <div className="text-xs text-(--tone)/80">{desc}</div>
        {chip && (
          <span className="mt-2 inline-flex items-center gap-1 rounded-md border border-border/70 bg-background/40 px-1.5 py-0.5 text-[11px] text-muted-foreground">
            <KeyRound className="size-3 opacity-80" aria-hidden />
            {chip}
          </span>
        )}
      </div>
    </div>
  );
}

function When({ at }: { at: string }) {
  return (
    <time dateTime={at} title={formatDateTime(at)} className="font-medium text-foreground tabular-nums">
      {formatRelative(at)}
    </time>
  );
}

/**
 * Last registry check and last recorded update. Unknown values say so: a
 * never checked image is a warning, a missing update run only a note.
 * `local` images cannot be checked at all.
 */
function TimesList({ times, local, showUpdated }: { times: UpdateTimes; local?: boolean; showUpdated: boolean }) {
  const t = useTranslations("common.updates");
  return (
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1.5 border-t border-border/50 px-4 py-3 text-[11.5px]" data-slot="update-times">
      <dt className="text-muted-foreground">{t("lastChecked")}</dt>
      <dd className="min-w-0 text-right" data-time="checked">
        {times.checkedAt ? (
          <>
            <When at={times.checkedAt} />
            {times.unchecked > 0 && (
              <span data-tone="warn" className="text-(--tone)">
                {" "}
                · {t("partlyUnchecked", { count: times.unchecked })}
              </span>
            )}
          </>
        ) : local ? (
          <span className="text-muted-foreground">{t("checkNotPossible")}</span>
        ) : (
          <span data-tone="warn" className="font-medium text-(--tone)">
            {t("neverChecked")}
          </span>
        )}
      </dd>
      {showUpdated && (
        <>
          <dt className="text-muted-foreground">{t("lastUpdated")}</dt>
          <dd className="min-w-0 text-right" data-time="updated">
            {times.updatedAt ? <When at={times.updatedAt} /> : <span className="text-muted-foreground">{t("noUpdateRecorded")}</span>}
          </dd>
        </>
      )}
    </dl>
  );
}

function VersionRow({ label, image, tone, latest }: { label: string; image: ImageVersion; tone?: string; latest?: boolean }) {
  const t = useTranslations("common.updates");
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="inline-flex items-center gap-1.5 text-muted-foreground">
        {latest ? <ArrowRight className="size-3" aria-hidden /> : <Box className="size-3" aria-hidden />}
        {label}
      </span>
      <span className="flex min-w-0 items-center gap-1.5">
        {image.version && image.version !== image.tag && <span className="truncate font-mono text-[11px] text-muted-foreground">{image.tag}</span>}
        <span
          data-tone={tone ?? "gray"}
          className={cn("rounded-md px-2 py-0.5 font-mono text-sm font-semibold", tone ? "bg-(--tone)/14 text-(--tone)" : "bg-muted text-foreground")}
        >
          {image.version ?? (latest ? t("versionUnknown") : image.tag)}
        </span>
      </span>
    </div>
  );
}

function Footer({ actions, pending }: { actions: UpdateActions; pending: boolean }) {
  if (!actions.onUpdate && !actions.onRecheck && !actions.blockedReason) return null;
  return <ActionFooter actions={actions} pending={pending} />;
}

function ActionFooter({ actions, pending }: { actions: UpdateActions; pending: boolean }) {
  const install = useCapability(actions.updateOperation ?? "installContainerUpdate");
  const check = useCapability("checkUpdates");
  const t = useTranslations("common.updates");
  return (
    <div className="flex flex-col gap-2 border-t border-border/50 bg-muted/40 p-3">
      {pending && actions.onUpdate && !actions.blockedReason && (
        <Button size="sm" className="group w-full" onClick={actions.onUpdate} disabled={actions.busy || !install.available} title={install.reason}>
          {actions.busy ? <Spinner data-icon="inline-start" /> : <UPDATE_ICON.minor data-icon="inline-start" className="transition-transform group-hover:-translate-y-0.5" />}
          {actions.updateLabel ?? t("actionUpdateContainer")}
        </Button>
      )}
      {actions.blockedReason && <p className="text-center text-xs text-muted-foreground">{actions.blockedReason}</p>}
      {actions.onRecheck && (
        <Button size="sm" variant={pending ? "ghost" : "secondary"} className="group w-full" onClick={actions.onRecheck} disabled={actions.busy || !check.available} title={check.reason}>
          <RefreshCw data-icon="inline-start" className="spin-on-hover" />
          {t("actionRecheck")}
        </Button>
      )}
      {actions.extra}
    </div>
  );
}

/**
 * Rich panel for a single image (container or service). `updatedAt` is the
 * caller's recorded last update; without it only the last check is shown.
 */
export function UpdatePanel({ update, actions = {}, updatedAt }: { update: UpdateInfo; actions?: UpdateActions; updatedAt?: string | null }) {
  const t = useTranslations("common.updates");
  const copy = useUpdateCopy()(update);
  const tone = UPDATE_TONE[copy.visual];
  const Icon = UPDATE_ICON[copy.visual];
  const latest = latestOf(update);
  const registry = "registry" in update ? t("registry", { registry: update.registry }) : undefined;
  const pending = hasUpdate(update);
  return (
    <div className="text-xs">
      <Header tone={tone} icon={<Icon />} title={copy.title} desc={copy.desc} chip={registry} />
      <div className="flex flex-col gap-3 p-4">
        {update.state === "current" ? (
          <div className="flex flex-col items-center gap-2 text-center text-muted-foreground">
            <span className="flex items-center gap-2">
              {t("running")} <span className="rounded-md bg-muted px-2 py-0.5 font-mono text-sm font-semibold text-foreground">{update.current.version ?? update.current.tag}</span>
            </span>
            <span>{copy.note}</span>
          </div>
        ) : (
          <>
            <div className="flex flex-col gap-2">
              <VersionRow label={t("current")} image={update.current} />
              {latest && <VersionRow label={t("latest")} image={latest} tone={tone} latest />}
            </div>
            {latest?.published && <div className="text-right text-[11px] text-muted-foreground">{t("published", { date: formatDate(latest.published) })}</div>}
            {copy.note && (
              <div data-tone={tone} className="rounded-lg bg-(--tone)/10 p-3 text-center leading-relaxed font-medium text-(--tone)">
                {copy.note}
              </div>
            )}
          </>
        )}
      </div>
      <TimesList times={{ ...checkTimes([update]), updatedAt: updatedAt ?? undefined }} local={update.state === "local"} showUpdated={updatedAt !== undefined} />
      <Footer actions={actions} pending={pending} />
    </div>
  );
}

/**
 * Aggregated panel for a stack: one row per service with its versions, then
 * the last check and, when the caller knows the stack's history, the last update.
 */
export function StackUpdatePanel({ services, actions = {}, times }: { services: Pick<Service, "name" | "update">[]; actions?: UpdateActions; times?: UpdateTimes }) {
  const t = useTranslations("common.updates");
  const copyOf = useUpdateCopy();
  const summary = summarizeUpdates(services);
  const tone = UPDATE_TONE[summary.visual];
  const Icon = UPDATE_ICON[summary.visual];
  const pending = summary.pending > 0;
  const affected = services.filter((s) => hasUpdate(s.update)).length;
  const unchecked = services.filter((s) => s.update.state === "unchecked").length;
  return (
    <div className="text-xs">
      <Header
        tone={summary.errors && !pending ? "err" : tone}
        icon={<Icon />}
        title={pending ? t("titleStackPending", { count: summary.pending }) : summary.errors ? t("titleError") : unchecked ? t("titleStackUnchecked") : t("titleStackCurrent")}
        desc={pending ? t("descStackPending", { services: affected }) : unchecked ? t("descStackUnchecked", { count: unchecked }) : t("descStackCurrent", { count: services.length })}
      />
      <ul className="flex flex-col divide-y divide-border/50 px-4 py-1">
        {services.map((s) => {
          const copy = copyOf(s.update);
          const v = updateVisual(s.update);
          const latest = latestOf(s.update);
          const rowTone = UPDATE_TONE[v];
          const RowIcon = UPDATE_ICON[v];
          return (
            <li key={s.name} className="flex items-center gap-2 py-2.5">
              <RowIcon data-tone={rowTone} className="size-3.5 shrink-0 text-(--tone)" aria-hidden />
              <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
              <span className="flex min-w-0 shrink-0 items-center gap-1.5 font-mono text-[13px] font-semibold" data-slot="update-versions">
                <span className={cn("rounded-md px-1.5 py-0.5", latest ? "text-muted-foreground" : "bg-muted text-foreground")}>{s.update.current.version ?? s.update.current.tag}</span>
                {latest && (
                  <>
                    <ArrowRight className="size-3.5 text-muted-foreground" aria-hidden />
                    <span data-tone={rowTone} className="rounded-md bg-(--tone)/14 px-1.5 py-0.5 text-(--tone)">
                      {latest.version ?? t("chipRebuild")}
                    </span>
                  </>
                )}
              </span>
              <span className="sr-only">{copy.title}</span>
            </li>
          );
        })}
      </ul>
      {pending && (
        <p className="px-4 pb-3 text-[11px] leading-relaxed text-muted-foreground">{t("noteManual")}</p>
      )}
      <TimesList times={times ?? checkTimes(services.map((s) => s.update))} showUpdated={!!times} />
      <Footer actions={{ updateLabel: t("actionUpdateStack"), ...actions, updateOperation: "installStackUpdates" }} pending={pending} />
    </div>
  );
}

export function UpdateChip({ update }: { update: UpdateInfo }) {
  const t = useTranslations("common.updates");
  const v = updateVisual(update);
  const Icon = UPDATE_ICON[v];
  const latest = latestOf(update);
  const text =
    v === "current" ? t("chipCurrent") : v === "rebuild" ? (latest?.version ?? t("chipRebuild")) : v === "error" ? t("chipError") : v === "local" ? t("chipLocal") : v === "unchecked" ? t("chipUnchecked") : (latest?.version ?? "");
  return (
    <ToneBadge tone={UPDATE_TONE[v]} className="cursor-pointer">
      <Icon aria-hidden />
      {text}
    </ToneBadge>
  );
}

export function StackUpdateChip({ services }: { services: Pick<Service, "name" | "update">[] }) {
  const t = useTranslations("common.updates");
  const s = summarizeUpdates(services);
  const Icon = UPDATE_ICON[s.visual];
  const text = s.pending ? t("chipPending", { count: s.pending }) : s.errors ? t("chipError") : s.visual === "unchecked" ? t("chipUnchecked") : t("chipCurrent");
  return (
    <ToneBadge tone={UPDATE_TONE[s.visual]} className="cursor-pointer">
      <Icon aria-hidden />
      {text}
    </ToneBadge>
  );
}
