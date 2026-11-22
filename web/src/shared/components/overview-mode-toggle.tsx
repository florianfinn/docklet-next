import { LayoutGrid, Rows3, Rows4, SlidersHorizontal, Table2 } from "lucide-react";
import { useTranslations } from "use-intl";

import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
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
  const t = useTranslations("common.actions");
  return (
    <>
    {/* Phones: one menu instead of two rows of toggles (resource lists only). */}
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className="ml-auto md:hidden" data-slot="view-menu">
          <SlidersHorizontal data-icon="inline-start" />
          {t("view")}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        {layout && onLayout && (
          <DropdownMenuRadioGroup value={layout} onValueChange={(v) => onLayout(v as OverviewLayout)}>
            <DropdownMenuLabel>{labels.layout}</DropdownMenuLabel>
            <DropdownMenuRadioItem value="table">{labels.table}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="hosts">{labels.hosts}</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        )}
        {layout && onLayout && mode && onMode && <DropdownMenuSeparator />}
        {mode && onMode && (
          <DropdownMenuRadioGroup value={mode} onValueChange={(v) => onMode(v as OverviewMode)}>
            <DropdownMenuLabel>{labels.label}</DropdownMenuLabel>
            <DropdownMenuRadioItem value="compact">{labels.compact}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="advanced">{labels.advanced}</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
    <div data-slot="view-controls" className={cn("ml-auto flex max-w-full flex-wrap items-center justify-end gap-2 max-md:hidden", className)}>
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
    </>
  );
}
