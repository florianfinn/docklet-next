import type { InfoHoverCardProps } from "@/shared/component-set/types";
import { useExclusivePopup } from "@/shared/lib/popup-coordinator";
import { cn } from "@/shared/lib/utils";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@/shared/ui/hover-card";

/**
 * Free info hover card: tone-tinted header band with icon and title, body below.
 * The trigger is a real button (opens on hover and focus) and stops row navigation;
 * the popup coordinator keeps it exclusive with other info popups.
 */
export function InfoHoverCard({ trigger, icon, title, children, tone = "info", className }: InfoHoverCardProps) {
  const [open, setOpen] = useExclusivePopup();
  return (
    <HoverCard open={open} onOpenChange={setOpen} openDelay={250} closeDelay={150}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          data-row-stop
         
          className={cn("inline-flex rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)}
        >
          {trigger}
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-72 overflow-hidden rounded-xl p-0">
        <div data-slot="info-card" data-tone={tone}>
          <div className="flex items-center gap-2 border-b border-(--tone)/25 bg-(--tone)/10 px-3 py-2 text-sm font-medium text-(--tone) [&_svg]:size-4 [&_svg]:shrink-0">
            {icon}
            <span>{title}</span>
          </div>
          <div className="px-3 py-2.5 text-sm text-muted-foreground">{children}</div>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
