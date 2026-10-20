// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import type { ProgressItem } from "@/shared/component-set/types";
import { ProgressList } from "@/shared/component-set/progress-list";
import { Item, ItemContent, ItemDescription, ItemMedia, ItemTitle } from "@/shared/ui/item";
import { cn } from "@/shared/lib/utils";

/** Unknown executor knowledge has no measured progress; recorded output stays readable. */
export function RuntimeProgressList({ items }: { items: (ProgressItem & { unknown: boolean })[] }) {
  return <div className="flex flex-col gap-2">{items.map((item) => item.unknown ?
    <ol key={item.id} data-slot="progress-list"><Item asChild variant="outline" size="sm" data-tone={item.tone}
      className={cn("bg-card/60 transition-colors duration-(--dur-fast)", item.highlighted && "border-primary/60 ring-2 ring-primary/20")}>
      <li id={`run-${item.id}`} aria-current={item.highlighted || undefined}>
        {item.media && <ItemMedia>{item.media}</ItemMedia>}
        <ItemContent className="min-w-0"><ItemTitle className="w-full">{item.title}</ItemTitle><ItemDescription className="line-clamp-none">{item.description}</ItemDescription></ItemContent>
        {item.body && <div className="basis-full">{item.body}</div>}
      </li>
    </Item></ol> : <ProgressList key={item.id} items={[item]} />)}</div>;
}
