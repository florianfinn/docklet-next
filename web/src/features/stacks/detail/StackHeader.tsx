// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/stacks/detail/StackHeader.tsx
// Revision: f25cf36c7495738d1c83aa44c9e20da905b50eb78fe158af11f89d4fe57c0e09
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: M2 production shell: disconnected error handling, development-only demo boundary, neutral product text or responsive layout. Historical import recorded in web/PROVENANCE.md. Acceptance #85: preserve live inventory state, names, host isolation and accurate resource presentation. #126 guarded command mapping and recovery presentation; #110 observed transitions and identifiable current intents. #27 production text, source errors and capability boundaries. M10 #231: preserve progress text, scoped controls and localized timestamps. M14 #313–#317: authoritative origins, scoped runtime operations, retained replacement identities and hub stop-mode UI. Simple-user management, assigned-stack access and mode-aware self-service.

import { useIsUser } from "@/data/use-principal";
import { useStackCapability } from "@/shared/hooks/use-stack-capability";
import { useCapability } from "@/data/use-capability";
import { ChevronDown, FlaskConical, Globe, Layers, Play, RefreshCw, RotateCw, ScanSearch, Square, TriangleAlert } from "lucide-react";
import { useTranslations } from "use-intl";
import { useStackStopMode } from "../stop-mode";

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
  const command = useStackCapability(stack.id, stack.hostId);
  const user = useIsUser();
  const full = useCapability("saveStackFiles", { kind: "stack", id: stack.id });
  const redeploy = user ? full : command;
  const check = useCapability("checkUpdates", { kind: "stack", id: stack.id });
  const present = (start: () => Promise<string | undefined>) => void (inline ? startPresented({ stackId: stack.id }, "inline", start) : start());
  const stopMode = useStackStopMode();
  const stopText = useTranslations("stacks.stopMode");
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
            <StackUpdateIndicator name={stack.name} services={stack.services} times={updateTimes} actions={{ resourceRef: user ? { kind: "stack", id: stack.id } : undefined, onUpdate: () => present(() => cmd.installUpdates(stack.id)), onRecheck: () => void cmd.checkUpdates(stack.id), busy }} />
            <HiddenBadge kind="stacks" id={stack.id} name={stack.name} />
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <HostChip host={host} />
            <span>{t("servicesCount", { count: stack.serviceCount ?? stack.services.length })}</span>
            <span>{t("deployed", { time: stack.lastDeployAt ? formatRelative(stack.lastDeployAt) : t(stack.deployHistoryUnavailable ? "unknownDeploy" : "never") })}</span>
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
            <Button size="sm" variant="outline" disabled={busy || !command.available || !!stack.inventory?.stale} title={command.reason} onClick={() => void cmd.run(stack.id, "start")}>
              <Play data-icon="inline-start" /> {t("actions.start")}
            </Button>
          ) : (
            <>
              <HelpTarget topic="action.stop">
                <Button size="sm" variant="outline" disabled={busy || stopMode.unavailable || !command.available || !!stack.inventory?.stale} title={stopMode.unavailable ? stopText("modeUnavailable") : command.reason} onClick={() => void cmd.run(stack.id, "stop")}>
                  <Square data-icon="inline-start" /> {t("actions.stop")}
                </Button>
              </HelpTarget>
              <HelpTarget topic="action.restart">
                <Button size="sm" variant="outline" disabled={busy || !command.available || !!stack.inventory?.stale} title={command.reason} onClick={() => void cmd.run(stack.id, "restart")} className="group">
                  <RotateCw data-icon="inline-start" className="spin-on-hover" /> {t("actions.restart")}
                </Button>
              </HelpTarget>
            </>
          )}
          <div className="flex">
            <HelpTarget topic="action.redeploy">
              <Button size="sm" className="group rounded-r-none" disabled={busy || !redeploy.available || !!stack.inventory?.stale} title={redeploy.reason} onClick={() => present(() => cmd.run(stack.id, "redeploy"))}>
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
                  {stopMode.alternates.map((alternate) => <DropdownMenuItem key={alternate} disabled={!command.available || !!stack.inventory?.stale} title={command.reason} onSelect={() => void cmd.run(stack.id, alternate)}><Square /> {stopText(alternate)}</DropdownMenuItem>)}
                  <DropdownMenuItem disabled={!check.available} title={check.reason} onSelect={() => void cmd.checkUpdates(stack.id)}>
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
      {stack.definitionHint === "definition-invalid" && <Alert><TriangleAlert /><AlertDescription>{stopText("definitionInvalid")}</AlertDescription></Alert>}
      {host?.status === "offline" && (
        <Alert className="border-orange/30 bg-orange/8 text-orange">
          <TriangleAlert />
          <AlertDescription className="text-orange">{t("hostOffline", { host: host.name })}</AlertDescription>
        </Alert>
      )}
    </header>
  );
}
