import { MoreHorizontal } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { Button } from "@/shared/ui/button";
import { ContextMenu, ContextMenuContent, ContextMenuGroup, ContextMenuItem, ContextMenuLabel, ContextMenuSeparator, ContextMenuTrigger } from "@/shared/ui/context-menu";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
import { cn } from "@/shared/lib/utils";

export interface MenuAction {
  id: string;
  label: string;
  icon: ReactNode;
  onSelect: () => void;
  disabled?: boolean;
  title?: string;
  /** Demo-only entries are shown quieter. */
  muted?: boolean;
}

/** Menu entries in groups; the ellipsis button and the context menu render the same list. */
export type MenuGroups = readonly (readonly MenuAction[])[];

/** Entries now, or a hook that only an open menu calls, so closed rows read no descriptors. */
export type MenuSource = MenuGroups | { useGroups: () => MenuGroups };

const visible = (groups: MenuGroups) => groups.filter((g) => g.length > 0);
const PARTS = {
  dropdown: { Item: DropdownMenuItem, Group: DropdownMenuGroup, Separator: DropdownMenuSeparator },
  context: { Item: ContextMenuItem, Group: ContextMenuGroup, Separator: ContextMenuSeparator },
};

function Entries({ groups, kind }: { groups: MenuGroups; kind: keyof typeof PARTS }) {
  const { Item, Group, Separator } = PARTS[kind];
  return visible(groups).map((group, i) => (
    <Fragment key={group[0]?.id ?? i}>
      {(kind === "context" || i > 0) && <Separator />}
      <Group>
        {group.map((a) => (
          <Item key={a.id} data-action={a.id} title={a.title} disabled={a.disabled} onSelect={a.onSelect} className={cn(a.muted && "text-muted-foreground")}>
            {a.icon} {a.label}
          </Item>
        ))}
      </Group>
    </Fragment>
  ));
}

function LazyEntries({ useGroups, kind }: { useGroups: () => MenuGroups; kind: keyof typeof PARTS }) {
  return <Entries groups={useGroups()} kind={kind} />;
}

function Source({ source, kind }: { source: MenuSource; kind: keyof typeof PARTS }) {
  return "useGroups" in source ? <LazyEntries useGroups={source.useGroups} kind={kind} /> : <Entries groups={source} kind={kind} />;
}

/** Ellipsis button with the row's actions. */
export function ActionsMenu({ groups, label }: { groups: MenuSource; label: string }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="size-8" aria-label={label}>
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <Source source={groups} kind="dropdown" />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * Right click (or the context-menu key, or a long press on touch) on a row or
 * card opens the same actions as its ellipsis button. The trigger is the row
 * element itself; nested controls keep their own behaviour.
 */
export function ActionsContextMenu({ groups, title, children }: { groups: MenuSource | undefined; title: string; children: ReactNode }) {
  if (!groups) return children;
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-60" aria-label={title}>
        <ContextMenuLabel className="truncate">{title}</ContextMenuLabel>
        <Source source={groups} kind="context" />
      </ContextMenuContent>
    </ContextMenu>
  );
}
