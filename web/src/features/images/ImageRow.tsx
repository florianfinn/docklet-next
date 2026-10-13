import { Layers, Package, ShieldAlert } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatMem, formatRelative } from "@/domain/format";
import { imageReference, imageUsage, type DockerImage } from "@/domain/images";
import type { Host, Tone } from "@/domain/model";
import { ActionsMenu } from "@/shared/components/action-menu";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, type OverviewColumn } from "@/shared/components/overview/overview-table";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { UpdateIndicator } from "@/shared/components/updates/update-indicator";
import type { FitView } from "@/shared/lib/fit-view";
import { isNestedInteractive } from "@/shared/lib/row-navigation";

import { USAGE_TONE, useImageMenu } from "./image-menu";

/** Repository and tag as people read them; a dangling image names its former repository. */
export function ImageName({ image, className }: { image: DockerImage; className?: string }) {
  const t = useTranslations("images");
  if (image.tag === null)
    return (
      <span className={className}>
        <span className="text-muted-foreground italic">&lt;{t("noTag")}&gt;</span>
        <span className="block text-xs font-normal text-muted-foreground">{t("formerly", { repository: image.repository })}</span>
      </span>
    );
  return (
    <span className={className}>
      {image.repository}
      <span className="text-muted-foreground">:</span>
      <span className="font-mono text-[0.92em]">{image.tag}</span>
    </span>
  );
}

export function ConsumerLinks({ image, limit = 3 }: { image: DockerImage; limit?: number }) {
  const t = useTranslations("images");
  if (!image.consumers.length) return <span className="text-xs text-muted-foreground">{t("consumersNone")}</span>;
  const shown = image.consumers.slice(0, limit);
  return (
    <span className="flex flex-wrap items-center gap-1">
      {shown.map((c) =>
        c.system ? (
          <ToneBadge key={c.containerId} tone="gray" className="font-normal">
            {t("system")}
          </ToneBadge>
        ) : (
          <Link
            key={c.containerId}
            to={c.stackId ? `/stacks/${c.stackId}` : `/containers?c=${c.containerId}`}
            className="inline-flex items-center gap-1 rounded-md border bg-muted/30 px-1.5 py-0.5 text-xs outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          >
            {c.stackId ? <Layers className="size-3 text-muted-foreground" aria-hidden /> : <Package className="size-3 text-muted-foreground" aria-hidden />}
            {c.stackId ? `${c.stackId} · ${c.serviceName}` : c.name}
          </Link>
        ),
      )}
      {image.consumers.length > limit && <span className="text-xs text-muted-foreground">{t("consumersMore", { count: image.consumers.length - limit })}</span>}
    </span>
  );
}

/** Most severe non-zero finding count with its name; the full breakdown is in the label and the details. */
export function SecurityChip({ image }: { image: DockerImage }) {
  const t = useTranslations("images");
  const ts = useTranslations("security.severities");
  const v = image.vulnerabilities;
  if (!v) return <span className="text-xs text-muted-foreground">–</span>;
  const top = (["critical", "high", "medium", "low"] as const).find((k) => v[k] > 0);
  const tone: Tone = top === "critical" ? "err" : top === "high" ? "orange" : top === "medium" ? "warn" : "ok";
  const all = (["critical", "high", "medium", "low"] as const).map((k) => `${v[k]} ${ts(k)}`).join(", ");
  return (
    <Link to="/security?tab=images" data-tone={tone} aria-label={t("securityLabel", { all })} title={all} className="inline-flex items-center gap-1 rounded-md border border-(--tone)/30 bg-(--tone)/10 px-1.5 py-0.5 text-xs whitespace-nowrap text-(--tone) tabular-nums outline-none hover:bg-(--tone)/15 focus-visible:ring-2 focus-visible:ring-ring">
      <ShieldAlert className="size-3" aria-hidden />
      {top ? `${v[top]} ${ts(top)}` : t("securityClean")}
    </Link>
  );
}

interface ImageRowProps {
  image: DockerImage;
  host: Host | undefined;
  view: FitView;
  columns: OverviewColumn[];
  inBlock?: boolean;
  onOpen: (id: string) => void;
  onPull: (image: DockerImage) => void;
  onRemove: (image: DockerImage) => void;
}

/** One image as table row or card; the entry opens the details sheet. */
export const ImageRow = memo(function ImageRow({ image, host, view, columns, inBlock, onOpen, onPull, onRemove }: ImageRowProps) {
  const t = useTranslations("images");
  const tc = useTranslations("common.actions");
  const usage = imageUsage(image);
  const condensed = view === "condensed";
  const menu = useImageMenu(image, host, { details: onOpen, pull: onPull, remove: onRemove });
  const usageBadge = (
    <ToneBadge tone={USAGE_TONE[usage]} title={t(`usageHint.${usage}`)}>
      {t(`usage.${usage}`)}
    </ToneBadge>
  );
  const version = <span className="text-sm tabular-nums">{image.version ?? (image.tag ? <span className="font-mono text-xs">{image.tag}</span> : "–")}</span>;
  const cells: Record<string, ReactNode> = {
    name: (
      <div className="flex items-center gap-3">
        {!inBlock && view !== "cards" && (
          <IconTile color={host?.color} size="md">
            <Package />
          </IconTile>
        )}
        <div className="min-w-0">
          <button type="button" onClick={() => onOpen(image.id)} aria-label={t("rowOpen", { name: imageReference(image) })} className="block rounded-sm text-left text-sm font-semibold tracking-tight [overflow-wrap:anywhere] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring">
            <ImageName image={image} />
          </button>
          {(condensed || view === "cards") && (
            <span className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              {image.version && <span>{image.version}</span>}
              {condensed && usageBadge}
              {!inBlock && condensed && <HostChip host={host} />}
            </span>
          )}
        </div>
      </div>
    ),
    version,
    host: <HostChip host={host} />,
    usage: usageBadge,
    consumers: <ConsumerLinks image={image} />,
    size: <span className="font-mono text-xs tabular-nums">{formatMem(image.sizeMb)}</span>,
    created: <span className="text-xs text-muted-foreground">{formatRelative(image.createdAt)}</span>,
    update: image.update ? <UpdateIndicator name={imageReference(image)} update={image.update} /> : <span className="text-xs text-muted-foreground">–</span>,
    security: <SecurityChip image={image} />,
    meta: (
      <span className="flex flex-col gap-0.5 text-xs">
        <span className="font-mono tabular-nums">{formatMem(image.sizeMb)}</span>
        <span className="text-muted-foreground">{formatRelative(image.createdAt)}</span>
      </span>
    ),
  };
  return (
    <OverviewRowView
      view={view}
      columns={columns}
      cells={cells}
      menu={menu}
      menuTitle={imageReference(image)}
      pad={inBlock ? "py-2" : "py-2.5"}
      hostColor={host?.color}
      onClick={(e) => {
        if (!e.currentTarget.contains(e.target as Node) || isNestedInteractive(e.target, e.currentTarget)) return;
        onOpen(image.id);
      }}
      actions={<ActionsMenu groups={menu} label={`${tc("more")}: ${imageReference(image)}`} />}
    />
  );
});
