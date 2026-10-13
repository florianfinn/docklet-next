import { Cpu, Layers, MemoryStick, SquareArrowOutUpRight } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useMetrics } from "@/data/live";
import { formatMem } from "@/domain/format";
import type { ContainerInfo, Host } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { keepOpenForCoordinatedLayers } from "@/features/activity/layer-coordination";
import { HiddenBadge } from "@/shared/components/hidden-badge";
import { HostChip } from "@/shared/components/host-chip";
import { Meter } from "@/shared/components/meter";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { UpdatePanel } from "@/shared/components/updates/update-panel";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { Button } from "@/shared/ui/button";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Separator } from "@/shared/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";

import { ContainerFacts, ContainerProvenance } from "./ContainerFacts";
import { containerHref } from "./container-links";
import { OriginBadge } from "./OriginBadge";

interface InspectorProps {
  container: ContainerInfo | undefined;
  host: Host | undefined;
  onClose: () => void;
}

/**
 * Overview side sheet: status, resources, update and details. Start, stop,
 * restart and edit live on the container page (or the stack page); the sheet
 * links there. Non-modal like the action centre, so toasts stay usable above it.
 */
export function ContainerInspector({ container: c, host, onClose }: InspectorProps) {
  const t = useTranslations("containers.inspector");
  const to = useTranslations("containers.origin");
  const ts = useTranslations("common.status");
  const te = useTranslations("containers.errors");
  const cmd = useContainerCommands();
  const m = useMetrics((s) => (c ? s?.containers[c.id] : undefined));
  const rt = c ? containerRuntime(c, host) : "unknown";

  return (
    <Sheet modal={false} open={!!c} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        className="w-full gap-0 p-0 shadow-2xl sm:max-w-lg"
        data-layer="container-inspector"
        onInteractOutside={keepOpenForCoordinatedLayers}
        onOpenAutoFocus={(e) => {
          // Focus the dialog itself so the origin hover card does not pop open on entry.
          e.preventDefault();
          (e.currentTarget as HTMLElement).focus();
        }}
      >
        {c && (
          <>
            <SheetHeader className="border-b">
              <div className="flex items-center gap-3">
                <IconTile color={host?.color} size="lg">
                  <Layers />
                </IconTile>
                <div className="min-w-0">
                  <SheetTitle className="truncate">{c.name}</SheetTitle>
                  <SheetDescription className="flex flex-wrap items-center gap-2 pt-1">
                    <ToneBadge tone={RUNTIME_TONE[rt]}>{ts(rt)}</ToneBadge>
                    <OriginBadge origin={c.origin} note={c.originNote} />
                    <HostChip host={host} />
                    <HiddenBadge kind="containers" id={c.id} name={c.name} />
                  </SheetDescription>
                </div>
              </div>
            </SheetHeader>
            <ScrollArea className="min-h-0 flex-1">
              <div className="flex flex-col gap-5 p-4">
                <div className="flex flex-wrap items-center gap-2" data-slot="inspector-links">
                  {/* Navigation closes the sheet, so it never stays open over the next page. */}
                  <Button size="sm" asChild>
                    <Link to={containerHref(c.id)} onClick={onClose}>
                      <SquareArrowOutUpRight data-icon="inline-start" /> {t("openPage")}
                    </Link>
                  </Button>
                  {c.stackId && (
                    <Button size="sm" variant="outline" asChild>
                      <Link to={`/stacks/${c.stackId}`} onClick={onClose}>
                        <Layers data-icon="inline-start" /> {t("toStack")}
                      </Link>
                    </Button>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">{t("actionsOnPage")}</p>
                <ContainerProvenance c={c} />
                <section>
                  <div className="mb-2 flex items-center justify-between">
                    <h3 className="text-sm font-medium">{t("resources")}</h3>
                    <SimulationBadge />
                  </div>
                  <div className="grid grid-cols-2 gap-4 rounded-xl border p-3">
                    <Meter label="CPU" icon={<Cpu />} value={m?.cpu ?? 0} color={host?.color} />
                    <Meter label="RAM" icon={<MemoryStick />} value={m ? (m.memMb / Math.max(1, c.memLimitMb)) * 100 : 0} color={host?.color} sub={m ? `${formatMem(m.memMb)} / ${formatMem(c.memLimitMb)}` : undefined} />
                  </div>
                </section>
                <section className="overflow-hidden rounded-xl border">
                  <UpdatePanel
                    update={c.update}
                    actions={
                      c.origin === "unraid"
                        ? { onRecheck: () => void cmd.check(c.id), blockedReason: te("managed-by-unraid") }
                        : c.origin === "unknown"
                          ? { onRecheck: () => void cmd.check(c.id), blockedReason: to("unknownBody") }
                          : { onUpdate: () => void cmd.update(c.id), onRecheck: () => void cmd.check(c.id) }
                    }
                  />
                </section>
                <Separator />
                <ContainerFacts c={c} host={host} />
                <p className="text-xs text-muted-foreground">{t("envMasked")}</p>
              </div>
            </ScrollArea>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
