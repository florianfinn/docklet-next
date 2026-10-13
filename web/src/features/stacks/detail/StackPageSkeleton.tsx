import { Skeleton } from "@/shared/ui/skeleton";

/** Mirrors header, tab bar, services card and the three info cards. */
export function StackPageSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col">
      <div className="mb-5 flex items-start gap-4">
        <Skeleton className="size-11 rounded-xl" />
        <div className="flex flex-1 flex-col gap-2">
          <div className="flex gap-2">
            <Skeleton className="h-8 w-40" />
            <Skeleton className="h-6 w-24 rounded-lg" />
            <Skeleton className="h-6 w-20 rounded-lg" />
          </div>
          <Skeleton className="h-3.5 w-80 max-w-full" />
        </div>
        <Skeleton className="h-8 w-56 rounded-lg" />
      </div>
      <Skeleton className="mb-4 h-9 w-[420px] max-w-full rounded-lg" />
      <Skeleton className="h-[248px] rounded-xl" />
      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Skeleton className="h-[220px] rounded-xl" />
        <Skeleton className="h-[220px] rounded-xl" />
        <Skeleton className="h-[220px] rounded-xl" />
      </div>
    </div>
  );
}
