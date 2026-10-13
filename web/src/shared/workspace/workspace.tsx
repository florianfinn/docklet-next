import { Columns2, PanelRight, PanelRightClose, X } from "lucide-react";
import { createContext, useContext, useId, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export type AsideMode = "off" | "details" | "split";

/** Width of the workspace frame from which the aside column is offered. */
export const ASIDE_MIN_WIDTH = 1400;
const WideContext = createContext(false);

interface WorkspaceProps {
  label: string;
  /** Usually a FileTree. */
  sidebar: ReactNode;
  children: ReactNode;
  /**
   * Optional third column for wide screens: a details panel or a second file.
   * Rendered only while the surrounding WorkspaceFrame is at least ASIDE_MIN_WIDTH wide.
   */
  aside?: ReactNode;
  asideLabel?: string;
  /** "details" keeps the aside at a reading width; "split" gives a second editor the same share as the first. */
  asideKind?: "details" | "split";
  className?: string;
}

/**
 * Arcane workspace frame: file tree, editor pane and an optional aside in one
 * card. The aside only exists while the frame is wide, so phones and laptops
 * never pay for a hidden second panel or editor.
 */
export function Workspace({ label, sidebar, children, aside, asideLabel, asideKind = "details", className }: WorkspaceProps) {
  const wide = useContext(WideContext);
  const showAside = !!aside && wide;
  return (
    <section
      aria-label={label}
      data-slot="workspace"
      data-aside={showAside ? asideKind : "off"}
      data-glow
      className={cn(
        "grid overflow-hidden rounded-2xl border bg-card md:h-[min(76vh,880px)] md:grid-cols-[clamp(200px,16%,300px)_minmax(0,1fr)]",
        // Details read best at a bounded width; the editor takes the rest instead of leaving the aside half empty.
        showAside && asideKind === "details" && "md:grid-cols-[clamp(220px,12%,300px)_minmax(0,1fr)_clamp(340px,24%,30rem)]",
        showAside && asideKind === "split" && "md:grid-cols-[clamp(220px,12%,300px)_minmax(0,1fr)_minmax(0,1fr)]",
        className,
      )}
    >
      {sidebar}
      {children}
      {showAside && (
        <aside aria-label={asideLabel} data-slot="workspace-aside" className="flex min-h-0 min-w-0 flex-col border-l bg-card">
          {aside}
        </aside>
      )}
    </section>
  );
}

/** Toolbar plus Workspace; measures its own width to decide whether the aside fits. */
export function WorkspaceFrame({ children, className }: { children: ReactNode; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [wide, setWide] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => setWide(el.getBoundingClientRect().width >= ASIDE_MIN_WIDTH);
    update();
    const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(update);
    observer?.observe(el);
    return () => observer?.disconnect();
  }, []);
  return (
    <WideContext.Provider value={wide}>
      <div ref={ref} data-slot="workspace-frame" data-wide={wide || undefined} className={cn("@container/ws flex flex-col gap-3", className)}>
        {children}
      </div>
    </WideContext.Provider>
  );
}

/** Choice of what sits beside the editor; only offered while there is room for it. */
export function AsideToggle({ value, onChange, splitDisabled }: { value: AsideMode; onChange: (m: AsideMode) => void; splitDisabled?: boolean }) {
  const t = useTranslations("workspace.aside");
  const wide = useContext(WideContext);
  if (!wide) return null;
  return (
    <div className="flex" data-slot="aside-toggle">
      <SegmentedToggle
        value={splitDisabled && value === "split" ? "details" : value}
        onChange={onChange}
        label={t("label")}
        options={[
          { value: "off", label: t("off"), icon: <PanelRightClose />, hint: t("offHint") },
          { value: "details", label: t("details"), icon: <PanelRight />, hint: t("detailsHint") },
          ...(splitDisabled ? [] : [{ value: "split" as const, label: t("split"), icon: <Columns2 />, hint: t("splitHint") }]),
        ]}
      />
    </div>
  );
}

/** Header strip of an aside panel. */
export function AsideHeader({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border/70 bg-muted/25 px-3 text-xs font-medium text-muted-foreground">
      <span className="min-w-0 flex-1 truncate">{title}</span>
      {children}
    </div>
  );
}

export interface EditorTab {
  id: string;
  label: string;
  icon?: ReactNode;
  dirty?: boolean;
  closable?: boolean;
  /** Full path or other detail for the tooltip. */
  title?: string;
}

interface EditorPaneProps {
  label: string;
  tabs: EditorTab[];
  activeId: string;
  onSelect: (id: string) => void;
  onClose?: (id: string) => void;
  /** Buttons at the right end of the tab strip. */
  actions?: ReactNode;
  status?: ReactNode;
  children: ReactNode;
  bodyClassName?: string;
}

/** Open-file tabs, editor body and status bar of a workspace. */
export function EditorPane({ label, tabs, activeId, onSelect, onClose, actions, status, children, bodyClassName }: EditorPaneProps) {
  const t = useTranslations("workspace");
  const base = useId();
  const tabId = (id: string) => `${base}-tab-${tabs.findIndex((x) => x.id === id)}`;

  const onKeyDown = (e: KeyboardEvent) => {
    const active = tabs.find((x) => x.id === activeId);
    if (e.key === "Delete" && active?.closable && onClose) {
      e.preventDefault();
      onClose(active.id);
      return;
    }
    const step = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
    if (!step) return;
    const index = tabs.findIndex((x) => x.id === activeId);
    const next = tabs[(index + step + tabs.length) % tabs.length];
    if (!next) return;
    e.preventDefault();
    onSelect(next.id);
    document.getElementById(tabId(next.id))?.focus();
  };

  return (
    <div data-slot="editor-pane" className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-9 shrink-0 items-stretch border-b border-border/70 bg-muted/25 pr-1">
        <div role="tablist" aria-label={label} onKeyDown={onKeyDown} className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
          {tabs.map((tab) => {
            const selected = tab.id === activeId;
            return (
              <div key={tab.id} className={cn("relative flex shrink-0 items-stretch border-r border-border/70", selected && "bg-card before:absolute before:inset-x-0 before:top-0 before:h-0.5 before:bg-primary")}>
                <button
                  type="button"
                  role="tab"
                  id={tabId(tab.id)}
                  aria-selected={selected}
                  aria-controls={`${base}-panel`}
                  tabIndex={selected ? 0 : -1}
                  title={tab.title}
                  aria-keyshortcuts={tab.closable && onClose ? "Delete" : undefined}
                  aria-describedby={tab.closable && onClose ? `${base}-close-hint` : undefined}
                  onClick={() => onSelect(tab.id)}
                  className={cn("flex max-w-56 items-center gap-1.5 px-3.5 text-xs text-muted-foreground outline-none transition-colors focus-visible:bg-muted", selected && "text-foreground", tab.closable && onClose && "pr-1.5")}
                >
                  <span className="shrink-0 [&_svg]:size-3.5" aria-hidden>
                    {tab.icon}
                  </span>
                  <span className="truncate">{tab.label}</span>
                  {tab.dirty && <i className="size-1.5 shrink-0 rounded-full bg-primary" aria-label={t("unsaved")} />}
                  {/* The tablist may only contain tabs; keyboard and screen readers close with Delete. */}
                  {tab.closable && onClose && (
                    <span
                      aria-hidden
                      title={t("closeTab", { name: tab.label })}
                      onClick={(e) => {
                        e.stopPropagation();
                        onClose(tab.id);
                      }}
                      className="ml-1 grid size-5 shrink-0 cursor-pointer place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <X className="size-3" />
                    </span>
                  )}
                </button>
              </div>
            );
          })}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-0.5 pl-1">{actions}</div>}
        <span id={`${base}-close-hint`} className="sr-only">
          {t("closeHint")}
        </span>
      </div>
      <div role="tabpanel" id={`${base}-panel`} aria-labelledby={tabId(activeId)} className={cn("min-h-[360px] min-w-0 flex-1 overflow-hidden bg-code", bodyClassName)}>
        {children}
      </div>
      {status && <div className="flex min-h-8 shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-t border-border/70 px-3 py-1 text-xs text-muted-foreground">{status}</div>}
    </div>
  );
}

/** Icon button with tooltip for the tab strip of an editor pane. */
export function EditorAction({ label, icon, onClick, disabled }: { label: string; icon: ReactNode; onClick: () => void; disabled?: boolean }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon" className="size-8" onClick={onClick} disabled={disabled} aria-label={label}>
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** Centered message inside the editor body: loading, error or non-text file. */
export function EditorPlaceholder({ icon, title, description, action }: { icon: ReactNode; title: string; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="grid h-full place-items-center p-6">
      <div className="flex max-w-sm flex-col items-center gap-2 text-center">
        <span className="grid size-10 place-items-center rounded-xl border bg-card text-muted-foreground [&_svg]:size-5" aria-hidden>
          {icon}
        </span>
        <p className="text-sm font-medium">{title}</p>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        {action}
      </div>
    </div>
  );
}
