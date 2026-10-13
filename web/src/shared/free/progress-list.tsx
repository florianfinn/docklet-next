import { useId } from "react";

import { withPercent } from "@/domain/locale";
import type { ProgressItem, ProgressListProps } from "@/shared/component-set/types";
import { cn } from "@/shared/lib/utils";
import { Item, ItemActions, ItemContent, ItemDescription, ItemFooter, ItemMedia, ItemTitle } from "@/shared/ui/item";
import { Progress } from "@/shared/ui/progress";

function ProgressRow({ item }: { item: ProgressItem }) {
  const titleId = useId();
  const percent = Math.round(Math.min(100, Math.max(0, item.progress)));
  return (
    <Item
      asChild
      variant="outline"
      size="sm"
      data-tone={item.tone}
      className={cn("bg-card/60 transition-colors duration-(--dur-fast)", item.highlighted && "border-primary/60 ring-2 ring-primary/20")}
    >
      <li id={`run-${item.id}`} aria-current={item.highlighted || undefined}>
        {item.media && <ItemMedia>{item.media}</ItemMedia>}
        <ItemContent className="min-w-0">
          <ItemTitle id={titleId} className="w-full">
            {item.title}
          </ItemTitle>
          <ItemDescription className="line-clamp-none">{item.description}</ItemDescription>
        </ItemContent>
        {item.actions && <ItemActions>{item.actions}</ItemActions>}
        <ItemFooter className="gap-3">
          <Progress value={percent} aria-labelledby={titleId} className="h-1 flex-1 bg-muted *:data-[slot=progress-indicator]:bg-(--tone)" />
          <span className="w-10 text-right text-[11px] text-muted-foreground tabular-nums">{withPercent(percent)}</span>
        </ItemFooter>
        {item.body && <div className="basis-full">{item.body}</div>}
      </li>
    </Item>
  );
}

/** Free run list: one outlined card per item, progress with percentage underneath. */
export function ProgressList({ items, className }: ProgressListProps) {
  return (
    <ol data-slot="progress-list" className={cn("flex flex-col gap-2", className)}>
      {items.map((item) => (
        <ProgressRow key={item.id} item={item} />
      ))}
    </ol>
  );
}
