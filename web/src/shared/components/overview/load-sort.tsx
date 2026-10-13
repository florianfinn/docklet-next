import { useTranslations } from "use-intl";

import type { LoadMetric } from "@/shared/lib/load-sort";
import type { SortState } from "@/shared/lib/table-sort";

import { SegmentedToggle } from "../segmented-toggle";
import { SortButton } from "../sortable-head";

interface LoadSortHeadProps<K extends string> {
  loadKey: K;
  sort: SortState<K>;
  onSort: (key: K) => void;
  metric: LoadMetric;
  onMetric: (metric: LoadMetric) => void;
  label: string;
}

/**
 * Load column header: the button sorts by the chosen metric, the small CPU/RAM
 * choice beside it picks the metric. They are separate controls, so choosing a
 * metric never also flips the direction.
 */
export function LoadSortHead<K extends string>({ loadKey, sort, onSort, metric, onMetric, label }: LoadSortHeadProps<K>) {
  const t = useTranslations("common.overview");
  return (
    <span className="flex flex-wrap items-center gap-1" data-slot="load-sort-head">
      <SortButton sortKey={loadKey} sort={sort} onSort={onSort} label={t("loadSort", { metric: t(`metric.${metric}`) })}>
        {label}
      </SortButton>
      <LoadMetricToggle metric={metric} onMetric={onMetric} />
    </span>
  );
}

export function LoadMetricToggle({ metric, onMetric }: { metric: LoadMetric; onMetric: (metric: LoadMetric) => void }) {
  const t = useTranslations("common.overview");
  return (
    <SegmentedToggle<LoadMetric>
      size="xs"
      value={metric}
      onChange={onMetric}
      label={t("loadMetric")}
      options={[
        { value: "cpu", label: t("metric.cpu"), hint: t("metricHint.cpu") },
        { value: "mem", label: t("metric.mem"), hint: t("metricHint.mem") },
      ]}
    />
  );
}
