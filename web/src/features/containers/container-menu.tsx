import { Eye, Layers, Package, Pencil, Play, RotateCw, Square, SquareArrowOutUpRight } from "lucide-react";
import { useCapability } from "@/data/use-capability";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { imageHref } from "@/features/images/image-links";
import type { ContainerInfo } from "@/domain/model";
import type { MenuAction, MenuGroups } from "@/shared/components/action-menu";
import { useHideAction } from "@/shared/components/hide-action";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";

import { containerHref } from "./container-links";
import { useContainerRestart } from "./container-restart";

/** Actions of one container; the ellipsis button and the row's context menu show the same entries. */
export function useContainerMenu(c: ContainerInfo, open: { onOpen: (id: string) => void; onEdit: (id: string) => void }, hidden: boolean): MenuGroups {
  const t = useTranslations("containers");
  const navigate = useNavigate();
  const cmd = useContainerCommands();
  const command = useCapability("runContainerCommand", { kind: "container", id: c.id });
  const edit = useCapability("recreateContainer", { kind: "container", id: c.id });
  const hide = useHideAction("containers");
  const restart = useContainerRestart();
  const run: MenuAction[] =
    c.state === "running"
      ? [
          { id: "stop", label: t("menu.stop"), icon: <Square />, onSelect: () => void cmd.run(c.id, "stop") },
          { id: "restart", label: t("menu.restart"), icon: <RotateCw />, onSelect: () => restart([c]) },
        ]
      : [{ id: "start", label: t("menu.start"), icon: <Play />, onSelect: () => void cmd.run(c.id, "start") }];
  return [
    [
      { id: "details", label: t("menu.details"), icon: <Eye />, onSelect: () => open.onOpen(c.id) },
      { id: "page", label: t("menu.page"), icon: <SquareArrowOutUpRight />, onSelect: () => navigate(containerHref(c.id)) },
      ...(c.stackId ? [{ id: "stack", label: t("menu.stack"), icon: <Layers />, onSelect: () => navigate(`/stacks/${c.stackId}`) }] : []),
      { id: "image", label: t("menu.image"), icon: <Package />, onSelect: () => navigate(imageHref(c)) },
    ],
    [
      ...run.map((action) => ({ ...action, disabled: !command.available || !!c.inventory?.stale, title: command.reason })),
      ...(c.origin === "compose" ? [] : [{ id: "edit", title: edit.reason, label: t("menu.edit"), icon: <Pencil />, disabled: !edit.available || !!c.inventory?.stale, onSelect: () => open.onEdit(c.id) }]),

    ],
    [hide(c.id, c.name, hidden)],
  ];
}
