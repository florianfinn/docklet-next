import { ArrowDown, ArrowUp, ChevronsUpDown } from "lucide-react";
import type { ReactNode } from "react";

import { ariaSort, type SortState } from "@/shared/lib/table-sort";
import { cn } from "@/shared/lib/utils";
import { TableHead } from "@/shared/ui/table";

interface SortButtonProps<K extends string> {
  sortKey: K;
  sort: SortState<K>;
  onSort: (key: K) => void;
  children: ReactNode;
  /** Accessible hint, e.g. "Sortieren nach Name". */
  label: string;
  className?: string;
}

/** Header button of one sort key: a quiet highlight and an arrow mark the active key. */
export function SortButton<K extends string>({ sortKey, sort, onSort, children, label, className }: SortButtonProps<K>) {
  const active = sort.key === sortKey;
  const Icon = !active ? ChevronsUpDown : sort.dir === "asc" ? ArrowUp : ArrowDown;
  return (
    <button
      type="button"
      onClick={() => onSort(sortKey)}
      aria-label={label}
      data-sort-key={sortKey}
      data-active={active || undefined}
      className={cn(
        "group/sort inline-flex h-7 items-center gap-1 rounded-md px-1.5 text-left font-medium whitespace-nowrap text-muted-foreground outline-none transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
        active && "bg-primary/10 text-primary-text",
        className,
      )}
    >
      {children}
      <Icon className={cn("size-3.5 shrink-0", !active && "opacity-0 group-hover/sort:opacity-60 group-focus-visible/sort:opacity-60")} aria-hidden />
    </button>
  );
}

/**
 * Column header that sorts on click; aria-sort, a quiet highlight and an arrow
 * mark the active column. `filtered` adds a primary dot for a set column
 * filter; `filteredLabel` is its text for screen readers.
 */
export function SortableHead<K extends string>({ sortKey, sort, onSort, children, label, className, filtered, filteredLabel }: SortButtonProps<K> & { filtered?: boolean; filteredLabel?: string }) {
  return (
    <TableHead aria-sort={ariaSort(sort, sortKey)} data-sorted={sort.key === sortKey || undefined} data-filtered={filtered || undefined} className={cn("px-1", className)}>
      <SortButton sortKey={sortKey} sort={sort} onSort={onSort} label={filtered && filteredLabel ? `${label} (${filteredLabel})` : label}>
        {children}
        {filtered && <span data-slot="filter-marker" aria-hidden className="size-1.5 shrink-0 rounded-full bg-control-text" />}
      </SortButton>
    </TableHead>
  );
}
