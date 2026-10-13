import { Eye, List } from "lucide-react";
import { Fragment, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { useContainerMenu } from "@/features/containers/container-menu";
import { useStackMenu } from "@/features/stacks/overview/stack-menu";
import { useStacks } from "@/data/queries";
import type { Stack } from "@/domain/model";
import type { MenuAction, MenuGroups } from "@/shared/components/action-menu";
import { cn } from "@/shared/lib/utils";
import { useHiddenIds } from "@/shared/preferences/hidden-store";
import { ContextMenu, ContextMenuContent, ContextMenuGroup, ContextMenuItem, ContextMenuLabel, ContextMenuSeparator, ContextMenuTrigger } from "@/shared/ui/context-menu";

import { guardUnreachable, hrefOf } from "./fleet-segments";
import type { FleetUnit } from "./fleet-status";

const MEMBERS = 8;

function MenuItems({ groups }: { groups: MenuGroups }) {
  return groups
    .filter((g) => g.length > 0)
    .map((group, i) => (
      <Fragment key={group[0]?.id ?? i}>
        <ContextMenuSeparator />
        <ContextMenuGroup>
          {group.map((a) => (
            <ContextMenuItem key={a.id} data-action={a.id} disabled={a.disabled} onSelect={a.onSelect} className={cn(a.muted && "text-muted-foreground")}>
              {a.icon} {a.label}
            </ContextMenuItem>
          ))}
        </ContextMenuGroup>
      </Fragment>
    ));
}

function StackItems({ stack, unreachable }: { stack: Stack; unreachable: boolean }) {
  const hidden = useHiddenIds("stacks").includes(stack.id);
  return <MenuItems groups={guardUnreachable(useStackMenu(stack, hidden), unreachable)} />;
}

function ContainerItems({ unit, unreachable }: { unit: FleetUnit; unreachable: boolean }) {
  const navigate = useNavigate();
  const c = unit.containers[0]!;
  const hidden = useHiddenIds("containers").includes(c.id);
  const open = (id: string) => navigate(`/containers?c=${id}`);
  return <MenuItems groups={guardUnreachable(useContainerMenu(c, { onOpen: open, onEdit: open }, hidden), unreachable)} />;
}

/** Groups of several containers outside Compose: the filtered list and each member; no invented bulk command. */
function GroupItems({ unit }: { unit: FleetUnit }) {
  const t = useTranslations("visual.ring");
  const navigate = useNavigate();
  const list: MenuAction = { id: "list", label: t("openList"), icon: <List />, onSelect: () => navigate(hrefOf(unit)) };
  const members: MenuAction[] = unit.containers.slice(0, MEMBERS).map((c) => ({ id: `member:${c.id}`, label: t("openMember", { name: c.name }), icon: <Eye />, onSelect: () => navigate(`/containers?c=${c.id}`) }));
  return <MenuItems groups={[[list], members]} />;
}

function UnitItems({ unit }: { unit: FleetUnit }) {
  const stacks = useStacks();
  const unreachable = unit.state === "unreachable";
  const stack = unit.stackId ? stacks.data?.find((s) => s.id === unit.stackId) : undefined;
  if (unit.stackId && stacks.isPending) return null;
  if (stack) return <StackItems stack={stack} unreachable={unreachable} />;
  if (unit.kind === "group" && unit.containers.length === 1) return <ContainerItems unit={unit} unreachable={unreachable} />;
  return <GroupItems unit={unit} />;
}

interface SegmentMenuProps {
  unit: FleetUnit;
  title: string;
  onOpenChange?: (open: boolean) => void;
  onCloseAutoFocus?: (event: Event) => void;
  children: ReactNode;
}

/**
 * Right click, long press, the context-menu key or Shift+F10 on a ring
 * segment: the same actions and guards as the stack or container row menus.
 * Items are built only while the menu is open.
 */
export function SegmentMenu({ unit, title, onOpenChange, onCloseAutoFocus, children }: SegmentMenuProps) {
  const t = useTranslations("visual.ring");
  return (
    <ContextMenu onOpenChange={onOpenChange}>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-64" aria-label={t("menuLabel", { name: title })} data-segment-menu="" onCloseAutoFocus={onCloseAutoFocus}>
        <ContextMenuLabel className="truncate">{title}</ContextMenuLabel>
        {unit.state === "unreachable" && <ContextMenuLabel className="pt-0 font-normal text-destructive">{t("offlineNote")}</ContextMenuLabel>}
        <UnitItems unit={unit} />
      </ContextMenuContent>
    </ContextMenu>
  );
}
