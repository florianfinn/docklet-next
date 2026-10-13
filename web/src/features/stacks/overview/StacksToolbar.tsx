import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Search, Tag } from "lucide-react";
import { useTranslations } from "use-intl";

import { labelTone } from "@/domain/hosts";
import { ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import type { OverviewLayout, OverviewMode } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";

import { OverviewViewControls } from "@/shared/components/overview-mode-toggle";

import type { StatusFilter } from "./stack-filters";

interface ToolbarProps {
  query: string;
  onQuery: (q: string) => void;
  status: StatusFilter;
  onStatus: (s: StatusFilter) => void;
  labels: string[];
  selectedLabels: string[];
  onToggleLabel: (l: string) => void;
  onResetLabels: () => void;
  mode: OverviewMode;
  onMode: (m: OverviewMode) => void;
  layout: OverviewLayout;
  onLayout: (l: OverviewLayout) => void;
}

export function StacksToolbar(p: ToolbarProps) {
  const t = useTranslations("stacks");
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <InputGroup className="w-full sm:w-72">
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput type="search" placeholder={t("search")} aria-label={t("searchLabel")} value={p.query} onChange={(e) => p.onQuery(e.target.value)} />
        </InputGroup>
        <SegmentedToggle<StatusFilter> value={p.status} onChange={p.onStatus} label={t("statusFilter")} options={(["all", "running", "problem", "updates", "stopped"] as const).map((f) => ({ value: f, label: t(`filter.${f}`) }))} fit="content" wrap />
        <div className="flex-1" />
        <OverviewViewControls
          mode={p.mode}
          onMode={p.onMode}
          layout={p.layout}
          onLayout={p.onLayout}
          labels={{
            label: t("mode.label"),
            compact: t("mode.compact"),
            advanced: t("mode.advanced"),
            compactHint: t("mode.compactHint"),
            advancedHint: t("mode.advancedHint"),
            layout: t("layout.label"),
            table: t("layout.table"),
            hosts: t("layout.hosts"),
            tableHint: t("layout.tableHint"),
            hostsHint: t("layout.hostsHint"),
          }}
        />
      </div>
      {p.labels.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t("labelsFilter")}>
          <Tag className="size-3.5 text-muted-foreground" aria-hidden />
          {p.labels.map((l) => {
            const on = p.selectedLabels.includes(l);
            return (
              <button
                key={l}
                type="button"
                aria-pressed={on}
                onClick={() => p.onToggleLabel(l)}
                data-tone={labelTone(l)}
                className={cn(
                  "inline-flex h-7 items-center gap-1.5 rounded-lg border border-dashed px-2.5 text-xs text-muted-foreground transition-[background,color,border-color,transform] duration-(--dur-fast) hover:text-foreground active:scale-95",
                  on && "border-solid border-(--tone)/50 bg-(--tone)/12 text-(--tone)",
                )}
              >
                <ToneDot tone={labelTone(l)} className="size-1.5" />
                {l}
              </button>
            );
          })}
          {p.selectedLabels.length > 0 && (
            <Button variant="link" size="sm" className="h-7 text-xs" onClick={p.onResetLabels}>
              {t("resetLabels")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
