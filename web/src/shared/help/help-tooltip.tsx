import { Info } from "lucide-react";
import { Tooltip as TooltipPrimitive } from "radix-ui";
import { useRef, type ReactElement, type ReactNode, type RefObject } from "react";
import { useTranslations } from "use-intl";

import { useExclusivePopup } from "@/shared/lib/popup-coordinator";
import { cn } from "@/shared/lib/utils";
import { FieldLabel, FieldLegend } from "@/shared/ui/field";
import { Tooltip, TooltipTrigger } from "@/shared/ui/tooltip";

import { useHelpMode, type HelpMode } from "@/shared/preferences/help-store";
import type { deHelp } from "./messages/de";

type Topics = (typeof deHelp)["topics"];
/** Catalog key "group.topic", e.g. "settingsGeneral.timeZone". */
export type HelpKey = { [G in keyof Topics]: `${G & string}.${keyof Topics[G] & string}` }[keyof Topics];

/** Hover waits a moment so passing pointers do not cover the form; focus opens at once. */
export const HELP_OPEN_DELAY = 450;

type Side = "top" | "right" | "bottom" | "left";

const matches = (el: Element, selector: string) => {
  try {
    return el.matches(selector);
  } catch {
    return true;
  }
};

/**
 * Open state for a help popup. Focus opens it only when the focus is visible
 * (keyboard), so a dialog that focuses its first field's info button on open,
 * or focus returning after a click, shows nothing.
 */
function useHelpOpen(): { open: boolean; request: (next: boolean) => void; setOpen: (next: boolean) => void; trigger: RefObject<HTMLButtonElement | null> } {
  const [open, setOpen] = useExclusivePopup();
  const trigger = useRef<HTMLButtonElement>(null);
  const request = (next: boolean) => {
    const el = trigger.current;
    if (next && el && el === document.activeElement && !matches(el, ":focus-visible") && !matches(el, ":hover")) return;
    setOpen(next);
  };
  return { open, request, setOpen, trigger };
}

/** Explanation of one topic for a mode: title, text and impact in full, one sentence in compact. */
export function HelpBody({ topic, mode }: { topic: HelpKey; mode: Exclude<HelpMode, "off"> }) {
  const t = useTranslations("help");
  if (mode === "compact") return <p className="px-3 py-2 text-xs leading-relaxed text-popover-foreground">{t(`topics.${topic}.short`)}</p>;
  return (
    <div data-slot="help-body">
      <p className="flex items-center gap-1.5 border-b border-primary/25 bg-primary/10 px-3 py-1.5 text-xs font-semibold text-primary-text [&_svg]:size-3.5 [&_svg]:shrink-0">
        <Info aria-hidden />
        {t(`topics.${topic}.title`)}
      </p>
      <div className="flex flex-col gap-1.5 px-3 py-2 text-xs leading-relaxed">
        <p className="text-popover-foreground">{t(`topics.${topic}.full`)}</p>
        <p className="text-muted-foreground">
          <span className="font-medium text-foreground">{t("impact")}:</span> {t(`topics.${topic}.impact`)}
        </p>
      </div>
    </div>
  );
}

function HelpContent({ topic, mode, side }: { topic: HelpKey; mode: Exclude<HelpMode, "off">; side: Side }) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        data-help-content=""
        data-mode={mode}
        side={side}
        sideOffset={6}
        collisionPadding={8}
        className={cn(
          "z-50 overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-lg outline-none",
          "animate-in fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0",
          mode === "full" ? "w-[min(20rem,calc(100vw-1rem))]" : "max-w-[min(16rem,calc(100vw-1rem))]",
        )}
      >
        <HelpBody topic={topic} mode={mode} />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  );
}

interface HelpTooltipProps {
  topic: HelpKey;
  side?: Side;
  className?: string;
}

/**
 * Info button with an extended explanation. Hover opens after a short delay,
 * keyboard focus at once, a click or tap toggles it (touch has no hover), and
 * Escape or a pointer outside closes it. Exclusive with the info hover cards.
 * Renders nothing while explanations are off. Place it beside a label, never
 * inside a <label> or another button.
 */
export function HelpTooltip({ topic, side = "top", className }: HelpTooltipProps) {
  const t = useTranslations("help");
  const mode = useHelpMode();
  const { open, request, setOpen, trigger } = useHelpOpen();
  // Radix closes the tooltip on pointer down; the click then decides from the state before.
  const openBefore = useRef(false);
  if (mode === "off") return null;
  return (
    <Tooltip open={open} onOpenChange={request} delayDuration={HELP_OPEN_DELAY}>
      <TooltipTrigger asChild ref={trigger}>
        <button
          type="button"
          data-slot="help-trigger"
          data-row-stop
          aria-label={t("trigger", { title: t(`topics.${topic}.title`) })}
          onPointerDown={() => {
            openBefore.current = open;
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") openBefore.current = open;
          }}
          onClick={(e) => {
            e.preventDefault();
            // An explicit click or tap always toggles, also without hover (touch).
            setOpen(!openBefore.current);
          }}
          className={cn(
            "inline-grid size-5 shrink-0 place-items-center rounded-full text-muted-foreground transition-colors outline-none hover:text-control-text focus-visible:text-control-text focus-visible:ring-[3px] focus-visible:ring-ring/50 data-[state=delayed-open]:text-control-text data-[state=instant-open]:text-control-text [&_svg]:size-3.5",
            className,
          )}
        >
          <Info aria-hidden />
        </button>
      </TooltipTrigger>
      <HelpContent topic={topic} mode={mode} side={side} />
    </Tooltip>
  );
}

/**
 * Field label followed by its info button. The button sits beside the label
 * so it does not become part of the input's accessible name.
 */
export function HelpLabel({ topic, htmlFor, id, children, className }: { topic: HelpKey; htmlFor?: string; id?: string; children: ReactNode; className?: string }) {
  return (
    <div data-slot="help-label" className={cn("flex items-center gap-1", className)}>
      <FieldLabel htmlFor={htmlFor} id={id}>
        {children}
      </FieldLabel>
      <HelpTooltip topic={topic} />
    </div>
  );
}

/**
 * Uses an existing control (an action button) as the trigger, without an
 * extra icon: hover after the delay or keyboard focus explains the action;
 * its click stays the action. Do not wrap controls that already have their
 * own tooltip, or disabled buttons (they get no pointer events).
 */
export function HelpTarget({ topic, side = "top", children }: { topic: HelpKey; side?: Side; children: ReactElement }) {
  const mode = useHelpMode();
  const { open, request, trigger } = useHelpOpen();
  if (mode === "off") return children;
  return (
    <Tooltip open={open} onOpenChange={request} delayDuration={HELP_OPEN_DELAY}>
      <TooltipTrigger asChild ref={trigger}>
        {children}
      </TooltipTrigger>
      <HelpContent topic={topic} mode={mode} side={side} />
    </Tooltip>
  );
}

/** Legend of a group (radio cards, checkbox lists) with its info button; the group keeps its own aria-label. */
export function HelpLegend({ topic, children, className }: { topic: HelpKey; children: ReactNode; className?: string }) {
  return (
    <FieldLegend variant="label" className={cn("flex items-center gap-1", className)}>
      {children}
      <HelpTooltip topic={topic} />
    </FieldLegend>
  );
}
