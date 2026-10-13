import { LayoutGrid, Rows3, Rows4, Table2 } from "lucide-react";

import { cn } from "@/shared/lib/utils";
import type { OverviewLayout, OverviewMode } from "@/shared/preferences/preferences-store";

import { SegmentedToggle } from "./segmented-toggle";

export interface ViewControlLabels {
  label?: string;
  compact?: string;
  advanced?: string;
  compactHint?: string;
  advancedHint?: string;
  layout?: string;
  table?: string;
  hosts?: string;
  tableHint?: string;
  hostsHint?: string;
}

interface OverviewViewControlsProps {
  /** Omitted on pages without Compact/Advanced (volumes). */
  mode?: OverviewMode;
  onMode?: (m: OverviewMode) => void;
  layout?: OverviewLayout;
  onLayout?: (l: OverviewLayout) => void;
  labels: ViewControlLabels;
  className?: string;
}

/**
 * Layout (one table or host blocks) and Compact/Advanced toggles of the stack
 * and container overviews; Advanced adds the selection column. The mode toggle
 * stays the last control so its position does not move.
 */
export function OverviewViewControls({ mode, onMode, layout, onLayout, labels, className }: OverviewViewControlsProps) {
  return (
    <div data-slot="view-controls" className={cn("ml-auto flex max-w-full flex-wrap items-center justify-end gap-2", className)}>
      {layout && onLayout && (
        <SegmentedToggle
          value={layout}
          onChange={onLayout}
          label={labels.layout ?? ""}
          options={[
            { value: "table", label: labels.table ?? "", icon: <Table2 />, hint: labels.tableHint },
            { value: "hosts", label: labels.hosts ?? "", icon: <LayoutGrid />, hint: labels.hostsHint },
          ]}
        />
      )}
      {mode && onMode && (
        <SegmentedToggle
          value={mode}
          onChange={onMode}
          label={labels.label ?? ""}
          options={[
            { value: "compact", label: labels.compact ?? "", icon: <Rows3 />, hint: labels.compactHint },
            { value: "advanced", label: labels.advanced ?? "", icon: <Rows4 />, hint: labels.advancedHint },
          ]}
        />
      )}
    </div>
  );
}
