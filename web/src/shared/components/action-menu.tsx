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

const visible = (groups: MenuGroups) => groups.filter((g) => g.length > 0);

/** Ellipsis button with the row's actions. */
export function ActionsMenu({ groups, label }: { groups: MenuGroups; label: string }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="size-8" aria-label={label}>
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        {visible(groups).map((group, i) => (
          <Fragment key={group[0]?.id ?? i}>
            {i > 0 && <DropdownMenuSeparator />}
            <DropdownMenuGroup>
              {group.map((a) => (
                <DropdownMenuItem key={a.id} data-action={a.id} title={a.title} disabled={a.disabled} onSelect={a.onSelect} className={cn(a.muted && "text-muted-foreground")}>
                  {a.icon} {a.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * Right click (or the context-menu key, or a long press on touch) on a row or
 * card opens the same actions as its ellipsis button. The trigger is the row
 * element itself; nested controls keep their own behaviour.
 */
export function ActionsContextMenu({ groups, title, children }: { groups: MenuGroups | undefined; title: string; children: ReactNode }) {
  if (!groups) return children;
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-60" aria-label={title}>
        <ContextMenuLabel className="truncate">{title}</ContextMenuLabel>
        {visible(groups).map((group, i) => (
          <Fragment key={group[0]?.id ?? i}>
            <ContextMenuSeparator />
            <ContextMenuGroup>
              {group.map((a) => (
                <ContextMenuItem key={a.id} data-action={a.id} title={a.title} disabled={a.disabled} onSelect={a.onSelect} className={cn(a.muted && "text-muted-foreground")}>
                  {a.icon} {a.label}
                </ContextMenuItem>
              ))}
            </ContextMenuGroup>
          </Fragment>
        ))}
      </ContextMenuContent>
    </ContextMenu>
  );
}
