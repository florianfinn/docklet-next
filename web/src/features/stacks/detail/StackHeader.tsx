import { ChevronDown, FlaskConical, Globe, Layers, Play, RefreshCw, RotateCw, ScanSearch, Square, TriangleAlert } from "lucide-react";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import type { Host, StackDetail } from "@/domain/model";
import { HiddenBadge } from "@/shared/components/hidden-badge";
import { HostChip } from "@/shared/components/host-chip";
import { LabelChips } from "@/shared/components/label-chips";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { StackUpdateIndicator } from "@/shared/components/updates/update-indicator";
import { useStackUpdateTimes } from "@/shared/components/updates/update-times";
import { useAdapterText } from "@/data/use-adapter-text";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";

import { startPresented } from "@/features/activity/run-presentation";

import { StackStatusBadge } from "../StackStatusBadge";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";

/**
 * `inline`: the page below already shows deploy runs (Deployments tab), so
 * redeploys and updates started here open neither the action dialog nor a toast.
 */
export function StackHeader({ stack, host, inline = false }: { stack: StackDetail; host: Host | undefined; inline?: boolean }) {
  const t = useTranslations("stacks.detail");
  const dt = useAdapterText();
  const cmd = useStackCommands();
  const present = (start: () => Promise<string | undefined>) => void (inline ? startPresented({ stackId: stack.id }, "inline", start) : start());
  const busy = !!stack.busyActionId;
  const updateTimes = useStackUpdateTimes(stack);
  const allStopped = stack.services.every((s) => s.state === "stopped");
  return (
    <header className="mb-5 flex flex-col gap-3">
      <div className="flex flex-wrap items-start gap-4">
        <IconTile color={host?.color} size="lg">
          <Layers />
        </IconTile>
        <div className="min-w-[min(100%,320px)] flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{stack.name}</h1>
            <StackStatusBadge stack={stack} host={host} />
            <LabelChips labels={stack.labels} />
            <StackUpdateIndicator name={stack.name} services={stack.services} times={updateTimes} actions={{ onUpdate: () => present(() => cmd.installUpdates(stack.id)), onRecheck: () => void cmd.checkUpdates(stack.id), busy }} />
            <HiddenBadge kind="stacks" id={stack.id} name={stack.name} />
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <HostChip host={host} />
            <span>{t("servicesCount", { count: stack.services.length })}</span>
            <span>{t("deployed", { time: stack.lastDeployAt ? formatRelative(stack.lastDeployAt) : t("never") })}</span>
            {stack.url && (
              <ToneBadge tone="info" className="h-5 text-foreground">
                <Globe className="text-info" aria-hidden /> {stack.url.replace(/^https?:\/\//, "")}
              </ToneBadge>
            )}
            {stack.description && <span>· {dt(stack.description)}</span>}
          </div>
        </div>
        <div data-page-actions className="flex flex-wrap items-center gap-2 self-center">
          {allStopped ? (
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void cmd.run(stack.id, "start")}>
              <Play data-icon="inline-start" /> {t("actions.start")}
            </Button>
          ) : (
            <>
              <HelpTarget topic="action.stop">
                <Button size="sm" variant="outline" disabled={busy} onClick={() => void cmd.run(stack.id, "stop")}>
                  <Square data-icon="inline-start" /> {t("actions.stop")}
                </Button>
              </HelpTarget>
              <HelpTarget topic="action.restart">
                <Button size="sm" variant="outline" disabled={busy} onClick={() => void cmd.run(stack.id, "restart")} className="group">
                  <RotateCw data-icon="inline-start" className="spin-on-hover" /> {t("actions.restart")}
                </Button>
              </HelpTarget>
            </>
          )}
          <div className="flex">
            <HelpTarget topic="action.redeploy">
              <Button size="sm" className="group rounded-r-none" disabled={busy} onClick={() => present(() => cmd.run(stack.id, "redeploy"))}>
                <RefreshCw data-icon="inline-start" className={busy ? "animate-spin" : "spin-on-hover"} /> {t("actions.redeploy")}
              </Button>
            </HelpTarget>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" className="rounded-l-none border-l border-primary-foreground/20 px-2" disabled={busy} aria-label={t("redeployMenu")}>
                  <ChevronDown />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={() => void cmd.checkUpdates(stack.id)}>
                    <ScanSearch /> {t("actions.check")}
                  </DropdownMenuItem>
                </DropdownMenuGroup>
                {import.meta.env.DEV && <DropdownMenuSeparator />}
                {import.meta.env.DEV && <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={() => present(() => cmd.run(stack.id, "redeploy", "pull"))}>
                    <FlaskConical /> {t("actions.failDemo")}
                  </DropdownMenuItem>
                </DropdownMenuGroup>}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
      {host?.status === "offline" && (
        <Alert className="border-orange/30 bg-orange/8 text-orange">
          <TriangleAlert />
          <AlertDescription className="text-orange">{t("hostOffline", { host: host.name })}</AlertDescription>
        </Alert>
      )}
    </header>
  );
}
