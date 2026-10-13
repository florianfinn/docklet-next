import { useCallback, useEffect, useRef, type ReactNode } from "react";

import { useExclusivePopup } from "@/shared/lib/popup-coordinator";
import { cn } from "@/shared/lib/utils";
import { Popover, PopoverAnchor, PopoverContent } from "@/shared/ui/popover";

type OpenMode = "hover" | "keyboard" | "click";

const FOCUSABLE = "button:not([disabled]), a[href], input:not([disabled]), [tabindex]:not([tabindex='-1'])";

interface InteractiveHoverCardProps {
  /** Trigger content; rendered inside a real button. */
  children: ReactNode;
  content: ReactNode;
  label: string;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  openDelay?: number;
  closeDelay?: number;
  triggerClassName?: string;
  contentClassName?: string;
}

/**
 * HoverCard whose content stays usable: hover previews it, focus previews it,
 * Enter/Space/ArrowDown moves focus into it, Escape returns to the trigger and
 * a click pins it. Built on Popover because Radix HoverCard content is not
 * reachable by keyboard. Only one such card is open at a time.
 */
export function InteractiveHoverCard({
  children,
  content,
  label,
  side = "bottom",
  align = "start",
  openDelay = 220,
  closeDelay = 160,
  triggerClassName,
  contentClassName,
}: InteractiveHoverCardProps) {
  const mode = useRef<OpenMode>("hover");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const openTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const closeTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const clearTimers = useCallback(() => {
    clearTimeout(openTimer.current);
    clearTimeout(closeTimer.current);
  }, []);

  // Closed by another popup or a context change: no focus restoration, no reopen.
  const [open, setOpen] = useExclusivePopup(() => {
    clearTimers();
    mode.current = "hover";
  });

  useEffect(() => clearTimers, [clearTimers]);

  const show = useCallback(
    (next: OpenMode, delay = 0) => {
      clearTimers();
      const run = () => {
        mode.current = next;
        setOpen(true);
      };
      if (delay) openTimer.current = setTimeout(run, delay);
      else run();
    },
    [clearTimers, setOpen],
  );

  const hide = useCallback(
    (delay = 0) => {
      clearTimers();
      const run = () => setOpen(false);
      if (delay) closeTimer.current = setTimeout(run, delay);
      else run();
    },
    [clearTimers, setOpen],
  );

  const focusInside = () => {
    requestAnimationFrame(() => contentRef.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus());
  };

  return (
    <Popover open={open} onOpenChange={(o) => (o ? setOpen(true) : hide())}>
      <PopoverAnchor asChild>
        <button
          ref={triggerRef}
          type="button"
          data-row-stop
          aria-label={label}
          aria-haspopup="dialog"
          aria-expanded={open}
          className={cn(
            "inline-flex max-w-full items-center rounded-lg outline-none focus-visible:ring-[3px] focus-visible:ring-ring/60",
            triggerClassName,
          )}
          onPointerEnter={(e) => {
            if (e.pointerType === "mouse" && mode.current !== "click") show("hover", open ? 0 : openDelay);
          }}
          onPointerLeave={(e) => {
            if (e.pointerType === "mouse" && mode.current !== "click") hide(closeDelay);
          }}
          onFocus={(e) => {
            if (!open && e.currentTarget.matches(":focus-visible")) show("hover");
          }}
          onBlur={(e) => {
            if (contentRef.current?.contains(e.relatedTarget as Node | null)) return;
            if (mode.current !== "click") hide(closeDelay);
          }}
          onClick={(e) => {
            e.stopPropagation();
            if (open && mode.current === "click") hide();
            else show("click");
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " " || e.key === "ArrowDown") {
              e.preventDefault();
              e.stopPropagation();
              show("keyboard");
              focusInside();
            } else if (e.key === "Escape" && open) {
              e.stopPropagation();
              hide();
            }
          }}
        >
          {children}
        </button>
      </PopoverAnchor>
      <PopoverContent
        ref={contentRef}
        side={side}
        align={align}
        sideOffset={8}
        aria-label={label}
        className={cn("w-80 overflow-hidden rounded-2xl border-border/70 p-0 shadow-2xl", contentClassName)}
        onPointerEnter={() => clearTimers()}
        onPointerLeave={(e) => {
          if (e.pointerType !== "mouse" || mode.current === "click") return;
          if (contentRef.current?.contains(document.activeElement)) return;
          hide(closeDelay);
        }}
        onOpenAutoFocus={(e) => {
          if (mode.current !== "keyboard") e.preventDefault();
        }}
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          if (mode.current === "keyboard") triggerRef.current?.focus();
          mode.current = "hover";
        }}
        onInteractOutside={(e) => {
          if (triggerRef.current?.contains(e.target as Node)) e.preventDefault();
        }}
        onEscapeKeyDown={() => {
          mode.current = "keyboard";
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {content}
      </PopoverContent>
    </Popover>
  );
}
