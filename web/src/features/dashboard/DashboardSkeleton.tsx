import { Card } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

import type { WidgetSize } from "./dashboard-layout";
import { spanStyle, useWidgetGrid } from "./use-widget-grid";

/** Reserves the summary band and each widget's saved grid footprint so content does not jump. */
export function DashboardSkeleton({ sizes }: { sizes: WidgetSize[] }) {
  const { attach, columns, style } = useWidgetGrid();
  return (
    <div aria-busy="true">
      <Card className="mb-6 gap-5 p-5">
        <div className="flex flex-col items-center gap-6 lg:flex-row">
          <Skeleton className="size-[220px] rounded-full" />
          <div className="flex w-full min-w-0 flex-1 flex-col gap-3">
            <Skeleton className="h-3.5 w-48" />
            <div className="flex gap-3">
              <Skeleton className="size-11 rounded-full" />
              <div className="flex min-w-0 flex-1 flex-col gap-2">
                <Skeleton className="h-6 w-72 max-w-full" />
                <Skeleton className="h-4 w-96 max-w-full" />
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {Array.from({ length: 4 }, (_, i) => (
                <Skeleton key={i} className="h-8 w-28 rounded-lg" />
              ))}
            </div>
          </div>
        </div>
        <div className="flex gap-3 overflow-hidden">
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-[154px] min-w-[250px] flex-1 rounded-xl" />
          ))}
        </div>
      </Card>
      <div ref={attach} className="grid" style={style} data-slot="widget-grid-skeleton">
        {(sizes.length ? sizes : [{ w: 1, h: 4 }]).map((size, i) => (
          <Card key={i} className="h-full gap-3 overflow-hidden p-4" style={spanStyle(size, columns)}>
            <div className="flex items-center gap-2.5">
              <Skeleton className="size-7 rounded-lg" />
              <Skeleton className="h-4 w-32" />
            </div>
            {Array.from({ length: 5 }, (_, j) => (
              <Skeleton key={j} className="h-11 rounded-lg" />
            ))}
          </Card>
        ))}
      </div>
    </div>
  );
}
