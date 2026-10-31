import { ChevronDown, ChevronsUpDown, Server, Unplug } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { hostTone, isHostVisible, toggleHost } from "@/domain/hosts";
import type { Host, HostStatus } from "@/domain/model";
import { ToneDot } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { cn } from "@/shared/lib/utils";
import { setPreference, usePreference } from "@/shared/preferences/preferences-store";
import { useUiStyle, type HostFilterStyle } from "@/shared/preferences/ui-style-store";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";
import { SidebarGroup, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarMenuSkeleton } from "@/shared/ui/sidebar";

/** Status groups of the menu design: problems after online, each sorted by name. */
const STATUS_ORDER: HostStatus[] = ["online", "standby", "offline", "pending", "update-required", "token-rejected"];

interface FilterModel {
  hosts: Host[];
  selection: string[];
  shown: number;
  summary: string;
  isOn: (id: string) => boolean;
  toggle: (id: string) => void;
  only: (id: string) => void;
  all: () => void;
}

/** Selection logic shared by every design; hosts arrive sorted by name from useHostScope. */
function useFilterModel(): FilterModel & { isPending: boolean } {
  const t = useTranslations("shell.hostFilter");
  const { all, selection, isPending } = useHostScope();
  const ids = all.map((h) => h.id);
  const shown = all.filter((h) => isHostVisible(selection, h.id)).length;
  return {
    hosts: all,
    selection,
    shown,
    isPending,
    summary: selection.length ? t("summarySome", { shown, count: all.length }) : t("summaryAll", { count: all.length }),
    isOn: (id) => isHostVisible(selection, id),
    toggle: (id) => setPreference("hostSelection", toggleHost(selection, id, ids)),
    only: (id) => setPreference("hostSelection", ids.length === 1 ? [] : [id]),
    all: () => setPreference("hostSelection", []),
  };
}

function useHostLabel() {
  const t = useTranslations("shell.hostFilter");
  const tc = useTranslations("common.host");
  return (h: Host, on: boolean) => t("hostLabel", { name: h.name, status: tc(h.status), shown: String(on) });
}

/** Status in words for hosts that are not online; online hosts only show the dot. */
function StatusText({ host }: { host: Host }) {
  const tc = useTranslations("common.host");
  if (host.status === "online") return <ToneDot tone="ok" />;
  return (
    <span data-tone={hostTone(host)} className="inline-flex shrink-0 items-center gap-1 text-[11px] text-(--tone)">
      <ToneDot tone={hostTone(host)} pulse />
      {tc(host.status)}
    </span>
  );
}

/**
 * Host name as a toggle button in the host colour: selected hosts are tinted
 * and bold with a solid colour mark, hidden ones are faded and grey. Status is
 * separate from selection: a disconnected or pending host keeps its status
 * text (rows) or a red mark with an unplug icon (chips) in either state.
 */
function HostToggle({ host, on, label, onClick, variant, className, trailing }: { host: Host; on: boolean; label: string; onClick: () => void; variant: "row" | "chip"; className?: string; trailing?: React.ReactNode }) {
  const tc = useTranslations("common.host");
  const tone = hostTone(host);
  const problem = host.status !== "online";
  return (
    <button
      type="button"
      aria-pressed={on}
      aria-label={label}
      onClick={onClick}
      data-host-toggle={host.id}
      data-on={on || undefined}
      title={problem ? tc(host.status) : undefined}
      style={{ "--host": host.color } as React.CSSProperties}
      className={cn(
        "group/host-toggle flex min-w-0 items-center gap-2 border text-left outline-none transition-[background,border-color,color,opacity,transform] duration-(--dur-fast) focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.98]",
        variant === "row" ? "h-8 w-full rounded-lg px-2 text-[13px]" : "h-7 max-w-full rounded-full px-2.5 text-xs",
        on ? "border-(--host)/55 bg-(--host)/16 font-medium text-foreground hover:bg-(--host)/24" : "border-dashed border-border text-muted-foreground opacity-60 hover:opacity-100",
        className,
      )}
    >
      <span
        aria-hidden
        data-tone={tone}
        className={cn(
          "shrink-0 transition-colors duration-(--dur-fast)",
          variant === "row" ? "h-3.5 w-1.5 rounded-full" : "size-2 rounded-full",
          on ? "bg-(--host)" : "bg-muted-foreground/45",
          variant === "chip" && problem && "ring-2 ring-(--tone) ring-offset-1 ring-offset-sidebar",
        )}
      />
      <span className="min-w-0 flex-1 truncate">{host.name}</span>
      {variant === "chip" && problem && <Unplug data-tone={tone} className="size-3 shrink-0 text-(--tone)" aria-hidden />}
      {trailing}
    </button>
  );
}

function AllButton({ m }: { m: FilterModel }) {
  const t = useTranslations("shell.hostFilter");
  return (
    <button
      type="button"
      aria-pressed={m.selection.length === 0}
      onClick={m.all}
      className={cn("rounded-md px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground", m.selection.length === 0 && "bg-primary/15 text-primary-text")}
    >
      {t("all")}
    </button>
  );
}

/** Design 1: compact list of host toggles with an "only this" shortcut on hover or focus. */
function ListDesign({ m }: { m: FilterModel }) {
  const t = useTranslations("shell.hostFilter");
  const label = useHostLabel();
  return (
    <div role="group" aria-label={t("title")} className="mt-1 flex flex-col gap-1">
      {m.hosts.map((h) => {
        const on = m.isOn(h.id);
        return (
          <div key={h.id} className="group/host relative flex items-center">
            <HostToggle
              host={h}
              on={on}
              label={label(h, on)}
              onClick={() => m.toggle(h.id)}
              variant="row"
              trailing={
                <span className="transition-opacity group-focus-within/host:opacity-0 group-hover/host:opacity-0">
                  <StatusText host={h} />
                </span>
              }
            />
            <button
              type="button"
              onClick={() => m.only(h.id)}
              aria-label={t("onlyLabel", { name: h.name })}
              className="absolute right-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground opacity-0 transition-opacity outline-none hover:bg-background hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring group-hover/host:opacity-100 group-focus-within/host:opacity-100"
            >
              {t("only")}
            </button>
          </div>
        );
      })}
    </div>
  );
}

/** Design 2: wrapping chips in host colours; status as a red ring and unplug icon. */
function ChipsDesign({ m }: { m: FilterModel }) {
  const t = useTranslations("shell.hostFilter");
  const label = useHostLabel();
  return (
    <div role="group" aria-label={t("title")} className="mt-1.5 flex flex-wrap gap-1.5 px-0.5">
      {m.hosts.map((h) => {
        const on = m.isOn(h.id);
        return <HostToggle key={h.id} host={h} on={on} label={label(h, on)} onClick={() => m.toggle(h.id)} variant="chip" />;
      })}
    </div>
  );
}

/** Grouped host toggles used by the menu design and by the collapsed (icon) sidebar. */
function GroupedList({ m }: { m: FilterModel }) {
  const t = useTranslations("shell.hostFilter");
  const tc = useTranslations("common.host");
  const label = useHostLabel();
  const groups = STATUS_ORDER.map((status) => ({ status, hosts: m.hosts.filter((h) => h.status === status) })).filter((g) => g.hosts.length);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2 px-1">
        <span className="text-xs font-medium text-muted-foreground">{m.summary}</span>
        <AllButton m={m} />
      </div>
      {groups.map((g) => (
        <div key={g.status} role="group" aria-label={`${t("title")}: ${tc(g.status)}`}>
          <div data-tone={g.status === "standby" ? "gray" : g.status === "online" ? "ok" : g.status === "offline" ? "err" : "info"} className="mb-0.5 flex items-center gap-1.5 px-1.5 text-[11px] font-medium text-(--tone)">
            <ToneDot tone={g.status === "standby" ? "gray" : g.status === "online" ? "ok" : g.status === "offline" ? "err" : "info"} className="size-1.5" />
            {tc(g.status)} · {g.hosts.length}
          </div>
          <div className="flex flex-col gap-1">
            {g.hosts.map((h) => {
              const on = m.isOn(h.id);
              return <HostToggle key={h.id} host={h} on={on} label={label(h, on)} onClick={() => m.toggle(h.id)} variant="row" trailing={h.status === "online" ? undefined : <StatusText host={h} />} />;
            })}
          </div>
        </div>
      ))}
      <p className="px-1 text-[11px] leading-snug text-muted-foreground">{t("hint")}</p>
    </div>
  );
}

/** Stacked colour dots of the visible hosts; status rings mark hosts that are not online. */
function DotStack({ m }: { m: FilterModel }) {
  const visible = m.hosts.filter((h) => m.isOn(h.id));
  return (
    <span className="flex shrink-0 -space-x-1" aria-hidden>
      {visible.slice(0, 5).map((h) => (
        <i key={h.id} data-tone={hostTone(h)} className={cn("size-3 rounded-full border-2 border-sidebar", h.status !== "online" && "outline-2 outline-(--tone)")} style={{ background: h.color }} />
      ))}
    </span>
  );
}

/** Design 3: one compact trigger with the grouped list in a popover. */
function MenuDesign({ m }: { m: FilterModel }) {
  const t = useTranslations("shell.hostFilter");
  const problems = m.hosts.filter((h) => h.status !== "online").length;
  return (
    <Popover>
      <PopoverTrigger className="flex h-10 w-full items-center gap-2 rounded-xl border bg-background/60 px-2.5 text-left text-[13px] outline-none hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-sidebar-accent" aria-label={`${t("title")}: ${m.summary}`}>
        <DotStack m={m} />
        <span className="min-w-0 flex-1 truncate">{m.summary}</span>
        {problems > 0 && (
          <span data-tone="err" className="rounded-md bg-(--tone)/15 px-1.5 text-[11px] text-(--tone)">
            {t("problemHosts", { count: problems })}
          </span>
        )}
        <ChevronsUpDown className="size-3.5 text-muted-foreground" aria-hidden />
      </PopoverTrigger>
      <PopoverContent side="bottom" align="start" className="w-(--radix-popover-trigger-width) min-w-60 rounded-xl p-2">
        <GroupedList m={m} />
      </PopoverContent>
    </Popover>
  );
}

function Designs({ style, m }: { style: HostFilterStyle; m: FilterModel }) {
  if (style === "chips") return <ChipsDesign m={m} />;
  return <ListDesign m={m} />;
}

/**
 * Global host filter in the sidebar; empty selection means all hosts. The
 * design (list, chips, menu) is a design-lab choice; all edit the same
 * selection. The collapsed icon sidebar always offers the grouped menu.
 */
export function HostFilter() {
  const t = useTranslations("shell.hostFilter");
  const style = useUiStyle("hostFilterStyle");
  const open = usePreference("sidebarHostsOpen");
  const m = useFilterModel();
  const [iconOpen, setIconOpen] = useState(false);

  return (
    <SidebarGroup className="py-1" data-host-filter={style}>
      <div className="group-data-[collapsible=icon]:hidden">
        {style === "menu" ? (
          m.isPending ? (
            <SidebarMenuSkeleton showIcon />
          ) : (
            <MenuDesign m={m} />
          )
        ) : (
          <Collapsible open={open} onOpenChange={(o) => setPreference("sidebarHostsOpen", o)} className="rounded-xl border bg-background/60 p-1.5">
            <div className="flex items-center gap-1 px-1">
              <CollapsibleTrigger className="flex flex-1 items-center gap-1.5 rounded-md py-1 text-xs font-medium text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
                <ChevronDown className={cn("size-3.5 transition-transform duration-(--dur)", !open && "-rotate-90")} />
                {t("title")}
                {!open && <span className="ml-auto font-normal">{m.summary}</span>}
              </CollapsibleTrigger>
              {open && <AllButton m={m} />}
            </div>
            <CollapsibleContent className="data-[state=open]:animate-in data-[state=open]:fade-in-0">
              {m.isPending ? (
                <div className="mt-1 flex flex-col gap-0.5">
                  {Array.from({ length: 4 }, (_, i) => (
                    <SidebarMenuSkeleton key={i} showIcon />
                  ))}
                </div>
              ) : (
                <Designs style={style} m={m} />
              )}
              <p className="mt-1.5 px-1.5 text-[11px] leading-snug text-muted-foreground">{t("hint")}</p>
            </CollapsibleContent>
          </Collapsible>
        )}
      </div>
      <SidebarMenu className="hidden group-data-[collapsible=icon]:flex">
        <SidebarMenuItem>
          <Popover open={iconOpen} onOpenChange={setIconOpen}>
            <PopoverTrigger asChild>
              <SidebarMenuButton aria-label={`${t("title")}: ${m.summary}`} isActive={m.selection.length > 0}>
                <Server />
              </SidebarMenuButton>
            </PopoverTrigger>
            <PopoverContent side="right" align="start" className="w-64 rounded-xl p-2">
              <GroupedList m={m} />
            </PopoverContent>
          </Popover>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarGroup>
  );
}
