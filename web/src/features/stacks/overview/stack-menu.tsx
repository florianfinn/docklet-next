import { useCapability } from "@/data/use-capability";
import { Download, Eye, FileCode2, FileText, FlaskConical, Play, RefreshCw, RotateCw, ScanSearch, Square } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import type { Stack } from "@/domain/model";
import { summarizeUpdates } from "@/domain/updates";
import type { MenuAction, MenuGroups } from "@/shared/components/action-menu";
import { useHideAction } from "@/shared/components/hide-action";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";

/** Actions of one stack; the ellipsis button and the row's context menu show the same entries. */
export function useStackMenu(stack: Stack, hidden: boolean): MenuGroups {
  const t = useTranslations("stacks.menu");
  const navigate = useNavigate();
  const cmd = useStackCommands();
  const command = useCapability("runStackCommand", { kind: "stack", id: stack.id });
  const hide = useHideAction("stacks");
  const allStopped = stack.services.every((s) => s.state === "stopped");
  const pending = summarizeUpdates(stack.services).pending;
  const busy = !!stack.busyActionId;
  const href = `/stacks/${stack.id}`;
  const run: MenuAction[] = allStopped
    ? [{ id: "start", label: t("start"), icon: <Play />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "start") }]
    : [
        { id: "stop", label: t("stop"), icon: <Square />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "stop") },
        { id: "restart", label: t("restart"), icon: <RotateCw />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "restart") },
      ];
  return [
    [
      { id: "open", label: t("open"), icon: <Eye />, onSelect: () => navigate(href) },
      { id: "edit", label: t("edit"), icon: <FileCode2 />, onSelect: () => navigate(`${href}?tab=editor`) },
      { id: "logs", label: t("logs"), icon: <FileText />, onSelect: () => navigate(`${href}?tab=logs`) },
    ],
    [
      ...run,
      { id: "redeploy", label: t("redeploy"), icon: <RefreshCw />, disabled: busy || !command.available || !!stack.inventory?.stale, title: command.reason, onSelect: () => void cmd.run(stack.id, "redeploy") },
      ...(pending > 0 ? [{ id: "update", label: t("update"), icon: <Download />, disabled: busy, onSelect: () => void cmd.installUpdates(stack.id) }] : []),
      { id: "check", label: t("check"), icon: <ScanSearch />, disabled: busy, onSelect: () => void cmd.checkUpdates(stack.id) },
    ],
    [hide(stack.id, stack.name, hidden)],
    ...(import.meta.env.DEV ? [[{ id: "fail-demo", label: t("failDemo"), icon: <FlaskConical />, disabled: busy, muted: true, onSelect: () => void cmd.run(stack.id, "redeploy", "pull") }]] : []),
  ];
}
