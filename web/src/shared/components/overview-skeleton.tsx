import type { OverviewMode } from "@/shared/preferences/preferences-store";
import { Skeleton } from "@/shared/ui/skeleton";

/** Row skeletons sized like real rows (compact ≈ 72 px, advanced ≈ 44 px). */
export function OverviewSkeleton({ mode, rows = 7 }: { mode: OverviewMode; rows?: number }) {
  const compact = mode === "compact";
  return (
    <div className={compact ? "flex flex-col gap-2 pt-9" : "flex flex-col gap-1 pt-9"} aria-busy="true">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className={`flex items-center gap-4 rounded-xl border border-l-[3px] bg-card/40 px-3 ${compact ? "h-[72px]" : "h-[44px]"}`}>
          {compact && <Skeleton className="size-9 rounded-xl" />}
          <div className="flex min-w-48 flex-col gap-1.5">
            <Skeleton className={compact ? "h-4 w-32" : "h-3.5 w-28"} />
            {compact && <Skeleton className="h-3 w-48" />}
          </div>
          <Skeleton className="h-6 w-24 rounded-lg" />
          <Skeleton className="h-3.5 w-16" />
          <Skeleton className="h-5 w-12 rounded-md" />
          <Skeleton className="h-6 w-24 rounded-lg" />
          {!compact && (
            <>
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-3 w-14" />
              <Skeleton className="h-3 w-24" />
            </>
          )}
          <Skeleton className="ml-auto size-8 rounded-md" />
        </div>
      ))}
    </div>
  );
}
