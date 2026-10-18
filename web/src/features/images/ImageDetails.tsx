import { useCapability } from "@/data/use-capability";
import { ArrowRight, Download, Package, ShieldAlert, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatDateTime, formatMem, formatRelative } from "@/domain/format";
import { imageReference, imageUsage, shortDigest, type DockerImage } from "@/domain/images";
import type { Host } from "@/domain/model";
import { hasUpdate } from "@/domain/updates";
import { CopyButton } from "@/shared/components/code-block";
import { HostChip } from "@/shared/components/host-chip";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { UpdatePanel } from "@/shared/components/updates/update-panel";
import { Button } from "@/shared/ui/button";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Separator } from "@/shared/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";

import { USAGE_TONE } from "./image-menu";
import { ImageName } from "./ImageRow";

function Row({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={mono ? "min-w-0 font-mono text-xs [overflow-wrap:anywhere]" : "min-w-0 [overflow-wrap:anywhere]"}>{children}</dd>
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  );
}

interface ImageDetailsProps {
  /** Requested id; an id without image (removed meanwhile) shows a note instead of closing silently. */
  id: string | null;
  image: DockerImage | undefined;
  host: Host | undefined;
  onClose: () => void;
  onPull: (image: DockerImage) => void;
  onRemove: (image: DockerImage) => void;
}

/** Side sheet: human-readable version first, consumers and update/security links; the digest only here. */
export function ImageDetails({ id, image, host, onClose, onPull, onRemove }: ImageDetailsProps) {
  const t = useTranslations("images");
  const td = useTranslations("images.details");
  const pull = useCapability("pullImage");
  const remove = useCapability("removeImage");
  const online = host?.status === "online";
  // Where an update is installed: the first user workload of this image (never the agent).
  const user = image?.consumers.find((c) => !c.system);
  return (
    <Sheet open={!!id} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full gap-0 p-0 sm:max-w-xl" data-surface="image-details">
        {!image ? (
          <SheetHeader>
            <SheetTitle>{t("title")}</SheetTitle>
            <SheetDescription>{td("notFound")}</SheetDescription>
          </SheetHeader>
        ) : (
          <>
            <SheetHeader className="border-b">
              <div className="flex items-center gap-3">
                <IconTile color={host?.color} size="lg">
                  <Package />
                </IconTile>
                <div className="min-w-0">
                  <SheetTitle className="[overflow-wrap:anywhere]">
                    <ImageName image={image} />
                  </SheetTitle>
                  <SheetDescription className="flex flex-wrap items-center gap-2 pt-1">
                    <ToneBadge tone={USAGE_TONE[imageUsage(image)]}>{t(`usage.${imageUsage(image)}`)}</ToneBadge>
                    {image.version && <span className="text-sm text-foreground">{image.version}</span>}
                    <HostChip host={host} />
                  </SheetDescription>
                </div>
              </div>
            </SheetHeader>
            <ScrollArea className="min-h-0 flex-1">
              <div className="flex flex-col gap-5 p-4">
                {!online && <p className="rounded-lg border border-warning/40 bg-warning/10 p-2.5 text-xs text-warning">{td("offlineBlocked")}</p>}
                <Section title={td("overview")}>
                  <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm">
                    <Row label={td("reference")} mono>
                      {imageReference(image)}
                    </Row>
                    <Row label={td("version")}>{image.version ?? td("versionUnknown")}</Row>
                    <Row label={td("tags")} mono>
                      {image.tag === null ? `<${t("noTag")}>` : [image.tag, ...image.aliases].join(", ")}
                    </Row>
                    <Row label={td("host")}>
                      <HostChip host={host} />
                    </Row>
                    <Row label={td("architecture")} mono>
                      {image.metadataUnavailable ? td("unknown") : `${image.os}/${image.architecture}`}
                    </Row>
                    <Row label={td("created")}>
                      <span title={formatDateTime(image.createdAt)}>{formatRelative(image.createdAt)}</span>
                    </Row>
                    <Row label={td("pulled")}>{image.pulledAt ? formatRelative(image.pulledAt) : td("pulledNever")}</Row>
                    <Row label={td("size")}>{formatMem(image.sizeMb)}</Row>
                    <Row label={td("layers")}>{image.metadataUnavailable ? td("unknown") : image.layers}</Row>
                    <Row label={td("digest")} mono>
                      <span className="flex flex-wrap items-center gap-2">
                        <span title={image.digest}>{shortDigest(image.digest)}…</span>
                        <CopyButton text={image.digest} className="h-6 px-2 text-[11px]" />
                      </span>
                      <span className="mt-0.5 block font-sans text-[11px] text-muted-foreground">{td("digestHint")}</span>
                    </Row>
                  </dl>
                </Section>
                <Separator />
                <Section title={td("consumers")}>
                  {image.consumers.length ? (
                    <ul className="flex flex-col gap-1.5" data-slot="image-consumers">
                      {image.consumers.map((c) => (
                        <li key={c.containerId} className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/20 px-2.5 py-1.5 text-sm">
                          <span className="min-w-0 flex-1 font-medium [overflow-wrap:anywhere]">{c.system ? t("system") : c.name}</span>
                          {c.state !== "running" && <ToneBadge tone="gray">{td("stopped")}</ToneBadge>}
                          {!c.system && (
                            <Button asChild size="sm" variant="ghost" className="h-7 text-xs">
                              <Link to={c.stackId ? `/stacks/${c.stackId}` : `/containers?c=${c.containerId}`} onClick={onClose}>
                                {c.stackId ? `${c.stackName ?? c.stackId} · ${c.serviceName}` : t("menu.details")}
                                <ArrowRight data-icon="inline-end" />
                              </Link>
                            </Button>
                          )}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-muted-foreground">{td("consumersNone")}</p>
                  )}
                </Section>
                <Separator />
                <Section title={td("update")}>
                  {image.update ? (
                    <div className="overflow-hidden rounded-xl border">
                      <UpdatePanel update={image.update} />
                      {hasUpdate(image.update) && user && (
                        <div className="border-t bg-muted/30 p-2.5">
                          <Button asChild size="sm" variant="secondary" className="w-full">
                            <Link to={user.stackId ? `/stacks/${user.stackId}` : `/containers?c=${user.containerId}`} onClick={onClose}>
                              {user.stackId ? td("updateOpen") : td("updateContainer")}
                              <ArrowRight data-icon="inline-end" />
                            </Link>
                          </Button>
                        </div>
                      )}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">{td("updateNone")}</p>
                  )}
                </Section>
                <Separator />
                <Section title={td("security")}>
                  {image.vulnerabilities ? (
                    <div className="flex flex-wrap items-center gap-2">
                      {(["critical", "high", "medium", "low"] as const).map((k) => (
                        <ToneBadge key={k} tone={k === "critical" ? "err" : k === "high" ? "orange" : k === "medium" ? "warn" : "gray"} className="tabular-nums">
                          {k} {image.vulnerabilities?.[k]}
                        </ToneBadge>
                      ))}
                      <Button asChild size="sm" variant="ghost" className="h-7 text-xs">
                        <Link to="/security?tab=images" onClick={onClose}>
                          <ShieldAlert data-icon="inline-start" />
                          {td("securityOpen")}
                        </Link>
                      </Button>
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">{td("securityNone")}</p>
                  )}
                </Section>
                <Separator />
                <Section title={td("actions")}>
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" disabled={!pull.available || !!image.inventory?.stale || !online || image.tag === null} title={pull.reason} onClick={() => onPull(image)}>
                      <Download data-icon="inline-start" />
                      {t("menu.pullAgain")}
                    </Button>
                    <Button size="sm" variant="outline" className="text-destructive hover:text-destructive" disabled={!remove.available || !!image.inventory?.stale || !online || image.consumers.length > 0} title={remove.reason} onClick={() => onRemove(image)}>
                      <Trash2 data-icon="inline-start" />
                      {t("menu.remove")}
                    </Button>
                  </div>
                  {image.consumers.length > 0 && <p className="text-xs text-muted-foreground">{td("removeBlocked", { count: image.consumers.length })}</p>}
                </Section>
              </div>
            </ScrollArea>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
