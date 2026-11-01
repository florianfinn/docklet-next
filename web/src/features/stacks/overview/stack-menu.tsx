// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/stacks/overview/stack-menu.tsx
// Revision: 4f3bdebc11c000b74cc60f3d5e648c846e3a36051286b245607bcbd4c16fc8b8
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: M2 production shell: disconnected error handling, development-only demo boundary, neutral product text or responsive layout. Historical import recorded in web/PROVENANCE.md. #126 guarded command mapping and recovery presentation; #110 observed transitions and identifiable current intents. #27 production text, source errors and capability boundaries. M10 #231: production stack updates and lifecycle UI. M14 #313–#317: authoritative origins, scoped runtime operations, retained replacement identities and hub stop-mode UI. Simple-user management, assigned-stack access and mode-aware self-service.

import { useIsUser } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";
import { useStackCapability } from "@/shared/hooks/use-stack-capability";
import { useCapability } from "@/data/use-capability";
import { Download, Eye, FileCode2, FileText, FlaskConical, Play, RefreshCw, RotateCw, ScanSearch, Square } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import type { Stack } from "@/domain/model";
import { summarizeUpdates } from "@/domain/updates";
import type { MenuAction, MenuGroups } from "@/shared/components/action-menu";
import { useHideAction } from "@/shared/components/hide-action";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { useStackStopMode } from "../stop-mode";

/** Actions of one stack; the ellipsis button and the row's context menu show the same entries. */
export function useStackMenu(stack: Stack, hidden: boolean): MenuGroups {
  const t = useTranslations("stacks.menu");
  const navigate = useNavigate();
  const cmd = useStackCommands();
  const live = useAdapter().kind === "http";
  const user = useIsUser();
  const ref = user ? { kind: "stack" as const, id: stack.id } : undefined;
  const edit = useCapability("saveStackFiles", { kind: "stack", id: stack.id });
  const update = useCapability("installStackUpdates", ref);
  const check = useCapability("checkUpdates", ref);
  const command = useStackCapability(stack.id, stack.hostId);
  const hide = useHideAction("stacks");
  const allStopped = user && stack.runningCount !== undefined ? stack.runningCount === 0 : stack.services.every((s) => s.state === "stopped");
  const pending = summarizeUpdates(stack.services).pending;
  const stopMode = useStackStopMode();
  const stopText = useTranslations("stacks.stopMode");
  const busy = !!stack.busyActionId;
  const href = `/stacks/${stack.id}`;
  const run: MenuAction[] = allStopped
    ? [{ id: "start", label: t("start"), icon: <Play />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "start") }]
    : [
        { id: "stop", label: t("stop"), icon: <Square />, disabled: busy || stopMode.unavailable || !command.available || !!stack.inventory?.stale, title: stopMode.unavailable ? stopText("modeUnavailable") : command.reason, onSelect: () => void cmd.run(stack.id, "stop") },
        { id: "restart", label: t("restart"), icon: <RotateCw />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "restart") },
      ];
  return [
    [
      { id: "open", label: t("open"), icon: <Eye />, onSelect: () => navigate(href) },
      { id: "edit", label: t("edit"), icon: <FileCode2 />, disabled: user && !edit.available, title: user ? edit.reason : undefined, onSelect: () => navigate(`${href}?tab=editor`) },
      { id: "logs", label: t("logs"), icon: <FileText />, onSelect: () => navigate(`${href}?tab=logs`) },
    ],
    [
      ...run,
      ...stopMode.alternates.map((alternate): MenuAction => ({ id: alternate, label: stopText(alternate), icon: <Square />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, alternate) })),
      { id: "redeploy", label: t("redeploy"), icon: <RefreshCw />, disabled: busy || !command.available || (user && !edit.available) || !!stack.inventory?.stale, title: user ? edit.reason : command.reason, onSelect: () => void cmd.run(stack.id, "redeploy") },
      ...(pending > 0 ? [{ id: "update", label: t("update"), icon: <Download />, disabled: busy || ((!live || user) && !update.available), title: update.reason, onSelect: () => void cmd.installUpdates(stack.id) }] : []),
      { id: "check", label: t("check"), icon: <ScanSearch />, disabled: busy || ((!live || user) && !check.available), title: check.reason, onSelect: () => live ? navigate(href) : void cmd.checkUpdates(stack.id) },
    ],
    [hide(stack.id, stack.name, hidden)],
    ...(import.meta.env.DEV ? [[{ id: "fail-demo", label: t("failDemo"), icon: <FlaskConical />, disabled: busy, muted: true, onSelect: () => void cmd.run(stack.id, "redeploy", "pull") }]] : []),
  ];
}
