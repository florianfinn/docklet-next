// Source: docklet-next/issues/449
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original shared-trigger tooltip and tap help composition using existing free primitives.

import { useState } from "react";
import { CircleHelp } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export function HelpTap({ label, help }: { label: string; help: string }) {
  const [open, setOpen] = useState(false);
  const [tooltip, setTooltip] = useState(false);
  // The popover's own close focus returns to the button after Escape or a second tap and leaves focus where an
  // outside press put it; the tooltip stays controlled and closed while the popover is open.
  return <Popover open={open} onOpenChange={setOpen}>
    <Tooltip open={!open && tooltip} onOpenChange={setTooltip}>
      <TooltipTrigger asChild><PopoverTrigger asChild>
        <Button type="button" variant="ghost" size="icon-xs" aria-label={label}><CircleHelp aria-hidden /></Button>
      </PopoverTrigger></TooltipTrigger>
      <TooltipContent className="max-w-xs">{help}</TooltipContent>
    </Tooltip>
    <PopoverContent aria-label={label} className="max-w-xs text-sm" onClick={(event) => event.stopPropagation()}>{help}</PopoverContent>
  </Popover>;
}
